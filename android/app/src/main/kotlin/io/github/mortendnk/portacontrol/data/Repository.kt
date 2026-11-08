package io.github.mortendnk.portacontrol.data

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.builtins.ListSerializer
import okhttp3.sse.EventSource

enum class Conn { Idle, Connecting, Live, Error }

/**
 * Live device states from the server (SSE stream with reconnect) and the
 * control calls with optimistic updates.
 */
class Repository(private val scope: CoroutineScope) {
    @Volatile
    var api: Api? = null
        private set

    private val _devices = MutableStateFlow<Map<String, Device>>(emptyMap())
    val devices: StateFlow<Map<String, Device>> = _devices.asStateFlow()

    private val _conn = MutableStateFlow(Conn.Idle)
    val conn: StateFlow<Conn> = _conn.asStateFlow()

    private val _role = MutableStateFlow("none")
    val role: StateFlow<String> = _role.asStateFlow()

    private val _notices = MutableSharedFlow<Notice>(extraBufferCapacity = 16)
    val notices: SharedFlow<Notice> = _notices.asSharedFlow()

    /** Bumped when lists (scenes, automation) changed on the server. */
    private val _changes = MutableStateFlow(0)
    val changes: StateFlow<Int> = _changes.asStateFlow()

    private var source: EventSource? = null
    private var loop: Job? = null
    private var foreground = false

    fun configure(url: String, key: String) {
        if (url.isBlank() || key.isBlank()) {
            stop()
            api = null
            _devices.value = emptyMap()
            return
        }
        val a = Api(url, key)
        if (api?.base == a.base) return
        api = a
        if (foreground) start()
    }

    /** Live stream while the app is visible. */
    fun setForeground(fg: Boolean) {
        foreground = fg
        if (fg) start() else stop()
    }

    private fun start() {
        val a = api ?: return
        if (loop?.isActive == true) return
        loop = scope.launch {
            runCatching { _role.value = a.me().role }
            var backoff = 1000L
            while (true) {
                _conn.value = Conn.Connecting
                val closed = kotlinx.coroutines.CompletableDeferred<Unit>()
                source = a.events(
                    onEvent = { handle(it) },
                    onOpen = {
                        _conn.value = Conn.Live
                        backoff = 1000L
                    },
                    onFailure = {
                        _conn.value = Conn.Error
                        closed.complete(Unit)
                    },
                )
                closed.await()
                source?.cancel()
                delay(backoff)
                backoff = (backoff * 2).coerceAtMost(30_000L)
            }
        }
    }

    private fun stop() {
        loop?.cancel()
        loop = null
        source?.cancel()
        source = null
        _conn.value = Conn.Idle
    }

    private fun handle(raw: String) {
        val e = runCatching { json.decodeFromString(LiveEvent.serializer(), raw) }.getOrNull() ?: return
        when (e.type) {
            "snapshot" -> e.data?.let { d ->
                runCatching { json.decodeFromJsonElement(ListSerializer(Device.serializer()), d) }.getOrNull()?.let { list ->
                    _devices.value = list.associateBy { it.id }
                }
            }
            "device" -> e.data?.let { d ->
                runCatching { json.decodeFromJsonElement(Device.serializer(), d) }.getOrNull()?.let { dev ->
                    _devices.update { it + (dev.id to dev) }
                }
            }
            "devices" -> scope.launch { reload() }
            "notice" -> e.data?.let { d ->
                runCatching { json.decodeFromJsonElement(Notice.serializer(), d) }.getOrNull()?.let { _notices.tryEmit(it) }
            }
            "scenes", "automation", "timers", "settings" -> _changes.update { it + 1 }
        }
    }

    suspend fun reload() {
        val a = api ?: return
        runCatching { a.devices() }.onSuccess { list -> _devices.value = list.associateBy { it.id } }
    }

    /** Applies c optimistically; returns an error message or null. */
    suspend fun control(id: String, c: Control): String? {
        val a = api ?: return "Nicht verbunden"
        val before = _devices.value[id]
        if (before?.state != null) {
            val optimistic = before.copy(state = before.state.apply(c))
            _devices.update { it + (id to optimistic) }
        }
        return try {
            val d = a.control(id, c)
            _devices.update { it + (d.id to d) }
            null
        } catch (e: Exception) {
            if (before != null) _devices.update { it + (id to before) }
            e.message ?: "Fehler"
        }
    }

    suspend fun setTimers(id: String, t: Timers): String? {
        val a = api ?: return "Nicht verbunden"
        return try {
            val out = a.timers(id, t)
            _devices.value[id]?.let { d -> _devices.update { it + (id to d.copy(timers = out)) } }
            null
        } catch (e: Exception) {
            e.message
        }
    }
}

/** The state as it will look after c (for the optimistic update). */
fun AcState.apply(c: Control): AcState = copy(
    power = c.power ?: power,
    mode = c.mode ?: mode,
    target = c.target ?: target,
    fan = c.fan ?: fan,
    swing = c.swing ?: swing,
    eco = c.eco ?: eco,
    turbo = c.turbo ?: turbo,
    sleep = c.sleep ?: sleep,
    purifier = c.purifier ?: purifier,
    freezeProtection = c.freezeProtection ?: freezeProtection,
    outdoorSilent = c.outdoorSilent ?: outdoorSilent,
    selfClean = c.selfClean ?: selfClean,
    ieco = c.ieco ?: ieco,
    sound = c.sound ?: sound,
    rate = c.rate ?: rate,
    displayOn = c.display ?: displayOn,
)
