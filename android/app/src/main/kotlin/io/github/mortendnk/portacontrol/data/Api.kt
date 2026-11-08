package io.github.mortendnk.portacontrol.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
import java.io.IOException
import java.util.concurrent.TimeUnit

val json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = false
    coerceInputValues = true
}

class ApiException(val status: Int, message: String) : IOException(message)

/** Client for one PortaControl server (base URL + API key). */
class Api(baseUrl: String, private val apiKey: String) {
    val base: String = normalize(baseUrl)

    private val http = OkHttpClient.Builder()
        .connectTimeout(6, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .build()

    private val streamHttp = http.newBuilder().readTimeout(0, TimeUnit.SECONDS).build()

    private fun request(path: String): Request.Builder =
        Request.Builder().url(base + "/api/v1" + path)
            .header("X-API-Key", apiKey)
            .header("X-Client", "Android")

    suspend fun raw(method: String, path: String, body: String? = null): String = withContext(Dispatchers.IO) {
        val rb = body?.toRequestBody("application/json".toMediaType())
        val req = request(path).method(method, rb ?: if (method == "POST" || method == "PUT") "".toRequestBody() else null).build()
        http.newCall(req).execute().use { res ->
            val text = res.body?.string().orEmpty()
            if (!res.isSuccessful) {
                val msg = runCatching { json.parseToJsonElement(text).let { (it as? kotlinx.serialization.json.JsonObject)?.get("error")?.toString()?.trim('"') } }.getOrNull()
                throw ApiException(res.code, msg ?: "HTTP ${res.code}")
            }
            text
        }
    }

    suspend inline fun <reified T> get(path: String): T = json.decodeFromString(raw("GET", path))

    suspend fun health(): Health = withContext(Dispatchers.IO) {
        val req = Request.Builder().url("$base/health").build()
        http.newCall(req).execute().use { res ->
            if (!res.isSuccessful) throw ApiException(res.code, "HTTP ${res.code}")
            json.decodeFromString(res.body?.string().orEmpty())
        }
    }

    suspend fun me(): Me = get("/auth/me")
    suspend fun devices(): List<Device> = get("/devices")
    suspend fun device(id: String): Device = get("/devices/$id")
    suspend fun control(id: String, c: Control): Device =
        json.decodeFromString(raw("POST", "/devices/$id/control", json.encodeToString(Control.serializer(), c)))
    suspend fun refresh(id: String): Device = json.decodeFromString(raw("POST", "/devices/$id/refresh"))
    suspend fun scenes(): List<Scene> = get("/scenes")
    suspend fun applyScene(id: String, device: String? = null) {
        raw("POST", "/scenes/$id/apply" + (device?.let { "?device=$it" } ?: ""))
    }
    suspend fun schedules(): List<Schedule> = get("/schedules")
    suspend fun saveSchedule(s: Schedule) {
        raw("PUT", "/schedules/${s.id}", json.encodeToString(Schedule.serializer(), s))
    }
    suspend fun rules(): List<Rule> = get("/rules")
    suspend fun saveRule(r: Rule) {
        raw("PUT", "/rules/${r.id}", json.encodeToString(Rule.serializer(), r))
    }
    suspend fun timers(id: String, t: Timers): Timers =
        json.decodeFromString(raw("PUT", "/devices/$id/timers", json.encodeToString(Timers.serializer(), t)))
    suspend fun energy(id: String, period: String, offset: Int): EnergyReply = get("/devices/$id/energy?period=$period&offset=$offset")
    suspend fun history(id: String, hours: Int): HistoryReply = get("/devices/$id/history?hours=$hours&points=400")
    suspend fun log(): List<LogEntry> = get("/log?limit=100")

    /** Server-sent events; the listener gets every "data:" payload. */
    fun events(onEvent: (String) -> Unit, onOpen: () -> Unit, onFailure: (Throwable?) -> Unit): EventSource {
        val req = request("/events").header("Accept", "text/event-stream").build()
        return EventSources.createFactory(streamHttp).newEventSource(req, object : EventSourceListener() {
            override fun onOpen(eventSource: EventSource, response: okhttp3.Response) = onOpen()
            override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) = onEvent(data)
            override fun onFailure(eventSource: EventSource, t: Throwable?, response: okhttp3.Response?) = onFailure(t)
            override fun onClosed(eventSource: EventSource) = onFailure(null)
        })
    }

    companion object {
        /** "192.168.1.5" → "http://192.168.1.5:8080"; full URLs stay as they are. */
        fun normalize(url: String): String {
            var u = url.trim().trimEnd('/')
            if (!u.startsWith("http://") && !u.startsWith("https://")) {
                val host = u.substringBefore('/')
                u = "http://" + if (host.contains(':')) u else host + ":8080" + u.removePrefix(host)
            }
            return u
        }

        /** portacontrol://connect?url=...&key=... from the QR code of the web UI. */
        fun parseLink(link: String): Pair<String, String>? {
            val l = link.trim()
            if (!l.startsWith("portacontrol://")) return null
            val q = l.substringAfter("?", "")
            val params = q.split("&").mapNotNull {
                val (k, v) = it.split("=", limit = 2).takeIf { p -> p.size == 2 } ?: return@mapNotNull null
                k to java.net.URLDecoder.decode(v, "UTF-8")
            }.toMap()
            val url = params["url"] ?: return null
            val key = params["key"] ?: return null
            return url to key
        }
    }
}
