package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Bedtime
import androidx.compose.material.icons.filled.CleaningServices
import androidx.compose.material.icons.filled.Eco
import androidx.compose.material.icons.filled.Monitor
import androidx.compose.material.icons.filled.PowerSettingsNew
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Bolt
import androidx.compose.material.icons.filled.AcUnit
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material.icons.filled.SwapVert
import androidx.compose.material.icons.automirrored.filled.VolumeOff
import androidx.compose.material.icons.automirrored.filled.VolumeUp
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Slider
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.Control
import io.github.mortendnk.portacontrol.data.Repository
import io.github.mortendnk.portacontrol.data.Scene
import io.github.mortendnk.portacontrol.data.Timers
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId
import kotlin.math.roundToInt

@OptIn(ExperimentalLayoutApi::class)
@Composable
fun DeviceScreen(repo: Repository, id: String, onBack: () -> Unit, snack: SnackbarHostState) {
    val devices by repo.devices.collectAsState()
    val changes by repo.changes.collectAsState()
    val d = devices[id]
    val scope = rememberCoroutineScope()
    var scenes by remember { mutableStateOf<List<Scene>>(emptyList()) }
    var confirmClean by remember { mutableStateOf(false) }

    LaunchedEffect(changes) { scenes = runCatching { repo.api?.scenes() }.getOrNull().orEmpty() }

    fun send(c: Control) {
        scope.launch { repo.control(id, c)?.let { snack.showSnackbar(it) } }
    }

    if (d == null) {
        Column(Modifier.fillMaxSize().padding(16.dp)) { Text(stringResource(R.string.connecting)) }
        return
    }
    val s = d.state
    val on = s?.power == true
    val av = d.available
    val enabled = s != null && !d.locked
    val color = if (on) modeColor(s?.mode) else MaterialTheme.colorScheme.onSurfaceVariant

    Column(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Filled.ArrowBack, null) }
            Column(Modifier.weight(1f)) {
                Text(d.name, fontSize = 20.sp, fontWeight = FontWeight.SemiBold, maxLines = 1)
                Text(
                    if (d.online) (if (d.path == "cloud") stringResource(R.string.via_cloud) else stringResource(R.string.via_lan)) else stringResource(R.string.unreachable),
                    fontSize = 12.sp,
                    color = if (d.online) Pc.ok else MaterialTheme.colorScheme.error,
                )
            }
            IconButton(onClick = {
                scope.launch {
                    runCatching { repo.api?.refresh(id) }.onFailure { snack.showSnackbar(it.message ?: "") }
                }
            }) { Icon(Icons.Filled.Refresh, stringResource(R.string.refresh)) }
        }

        if (!d.online && d.error != null) {
            Text(d.error, color = MaterialTheme.colorScheme.error, fontSize = 13.sp)
        }
        if (s?.filterAlert == true) Warn(stringResource(R.string.filter_alert))
        if ((s?.errorCode ?: 0) != 0) Warn(stringResource(R.string.error_code, s!!.errorCode))

        Section {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    if (on) stringResource(MODES.first { it.id == s?.mode }.label) else stringResource(R.string.off),
                    color = color, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f),
                )
                FilterChip(
                    selected = on, enabled = enabled, onClick = { send(Control(power = !on)) },
                    label = { Text(if (on) stringResource(R.string.on) else stringResource(R.string.off)) },
                    leadingIcon = { Icon(Icons.Filled.PowerSettingsNew, null, Modifier.size(18.dp)) },
                )
            }
            Thermostat(
                value = s?.target ?: 24.0,
                indoor = s?.indoor,
                color = modeColor(s?.mode),
                enabled = enabled && av.target,
                min = if (d.minTarget > 0) d.minTarget else 16.0,
                max = if (d.maxTarget > 0) d.maxTarget else 30.0,
                onChange = { send(Control(target = it)) },
            )
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                MODES.forEachIndexed { i, m ->
                    SegmentedButton(
                        selected = s?.mode == m.id,
                        enabled = enabled,
                        onClick = { send(if (on) Control(mode = m.id) else Control(mode = m.id, power = true)) },
                        shape = SegmentedButtonDefaults.itemShape(i, MODES.size),
                        icon = {},
                        label = { Icon(m.icon, stringResource(m.label), Modifier.size(20.dp)) },
                    )
                }
            }
        }

        Section {
            Row {
                StatBox(stringResource(R.string.indoor), temp(s?.indoor), Modifier.weight(1f))
                StatBox(stringResource(R.string.outdoor), temp(s?.outdoor), Modifier.weight(1f))
                StatBox(
                    stringResource(R.string.power), s?.watts?.let { num(it, 0) + " W" } ?: "–", Modifier.weight(1f),
                    sub = d.costPerHour?.let { money(it, d.today?.currency ?: "EUR") + "/h" },
                )
            }
            Row {
                StatBox(stringResource(R.string.today), d.today?.let { num(it.kwh, 2) + " kWh" } ?: "–", Modifier.weight(1f), sub = d.today?.let { money(it.cost, it.currency) })
                StatBox(stringResource(R.string.total), s?.totalKwh?.let { num(it, 1) + " kWh" } ?: "–", Modifier.weight(1f))
                StatBox(stringResource(R.string.runtime), d.today?.let { num(it.onHours, 1) + " h" } ?: "–", Modifier.weight(1f))
            }
            FanControl(s?.fan ?: 102, enabled && av.fan) { send(Control(fan = it)) }
        }

        Section(stringResource(R.string.comfort)) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp), maxItemsInEachRow = 3) {
                Tile(Icons.Filled.SwapVert, stringResource(R.string.swing), s?.swing, enabled) { send(Control(swing = !(s?.swing ?: false))) }
                Tile(Icons.Filled.Eco, stringResource(R.string.eco), s?.eco, enabled && av.eco) { send(Control(eco = !(s?.eco ?: false))) }
                Tile(Icons.Filled.Bolt, stringResource(R.string.turbo), s?.turbo, enabled && av.turbo) { send(Control(turbo = !(s?.turbo ?: false))) }
                Tile(Icons.Filled.Bedtime, stringResource(R.string.sleep), s?.sleep, enabled && av.sleep) { send(Control(sleep = !(s?.sleep ?: false))) }
                Tile(Icons.AutoMirrored.Filled.VolumeOff, stringResource(R.string.outdoor_silent), s?.outdoorSilent, enabled && av.outdoorSilent) {
                    send(Control(outdoorSilent = s?.outdoorSilent != true))
                }
                Tile(Icons.Filled.Speed, stringResource(R.string.ieco), s?.ieco, enabled) { send(Control(ieco = s?.ieco != true)) }
                Tile(Icons.Filled.AutoAwesome, stringResource(R.string.ionizer), s?.purifier, enabled) { send(Control(purifier = !(s?.purifier ?: false))) }
                Tile(Icons.Filled.AcUnit, stringResource(R.string.frost), s?.freezeProtection, enabled && av.freeze) {
                    send(Control(freezeProtection = !(s?.freezeProtection ?: false)))
                }
                Tile(Icons.Filled.CleaningServices, stringResource(R.string.self_clean), s?.selfClean, enabled) {
                    if (s?.selfClean == true) send(Control(selfClean = false)) else confirmClean = true
                }
                Tile(Icons.Filled.Monitor, stringResource(R.string.display), s?.displayOn, enabled) { send(Control(display = !(s?.displayOn ?: true))) }
                Tile(if (s?.sound == false) Icons.AutoMirrored.Filled.VolumeOff else Icons.AutoMirrored.Filled.VolumeUp, stringResource(R.string.beep), s?.sound, enabled) {
                    send(Control(sound = s?.sound != true))
                }
            }
            Text(stringResource(R.string.power_limit), fontSize = 14.sp)
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                listOf(50, 75, 100).forEachIndexed { i, v ->
                    SegmentedButton(
                        selected = (s?.rate ?: 100) == v, enabled = enabled,
                        onClick = { send(Control(rate = v)) },
                        shape = SegmentedButtonDefaults.itemShape(i, 3),
                        label = { Text(if (v == 100) stringResource(R.string.none) else "$v %") },
                    )
                }
            }
        }

        TimersSection(d.timers, scenes, enabled,
            onSet = { t -> scope.launch { repo.setTimers(id, t)?.let { snack.showSnackbar(it) } } },
            deviceOn = s?.onTimer, deviceOff = s?.offTimer, onDevice = { send(it) })

        if (scenes.isNotEmpty()) {
            Section(stringResource(R.string.scenes)) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    scenes.forEach { sc ->
                        OutlinedButton(onClick = {
                            scope.launch { runCatching { repo.api?.applyScene(sc.id, id) }.onFailure { snack.showSnackbar(it.message ?: "") } }
                        }) { Text(sc.name) }
                    }
                }
            }
        }
        Spacer(Modifier.height(24.dp))
    }

    if (confirmClean) {
        AlertDialog(
            onDismissRequest = { confirmClean = false },
            title = { Text(stringResource(R.string.self_clean)) },
            text = { Text(stringResource(R.string.self_clean_confirm)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmClean = false
                    send(Control(selfClean = true))
                }) { Text(stringResource(R.string.start)) }
            },
            dismissButton = { TextButton(onClick = { confirmClean = false }) { Text(stringResource(R.string.cancel)) } },
        )
    }
}

@Composable
private fun Warn(text: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(Icons.Filled.Warning, null, tint = Pc.warn, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(8.dp))
        Text(text, color = Pc.warn, fontSize = 13.sp)
    }
}

@Composable
private fun Tile(icon: ImageVector, label: String, on: Boolean?, enabled: Boolean, onClick: () -> Unit) {
    val active = on == true
    val bg = if (active) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.surfaceVariant
    val fg = if (active) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurface
    Column(
        Modifier
            .width(104.dp)
            .alpha(if (enabled) 1f else 0.4f)
            .background(bg, RoundedCornerShape(16.dp))
            .border(1.dp, if (active) Color.Transparent else MaterialTheme.colorScheme.outline, RoundedCornerShape(16.dp))
            .clickable(enabled = enabled, onClick = onClick)
            .padding(12.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        Icon(icon, null, tint = fg)
        Text(label, color = fg, fontSize = 13.sp, maxLines = 2)
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun FanControl(value: Int, enabled: Boolean, onChange: (Int) -> Unit) {
    var local by remember(value) { mutableFloatStateOf(if (value == 102) 50f else value.toFloat()) }
    val auto = value == 102
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row {
            Text(stringResource(R.string.fan), Modifier.weight(1f), fontSize = 14.sp)
            Text(if (auto) stringResource(R.string.auto) else "${local.roundToInt()} %", color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Slider(
            value = local, onValueChange = { local = it }, valueRange = 1f..100f, enabled = enabled,
            onValueChangeFinished = { onChange(local.roundToInt()) },
            modifier = Modifier.alpha(if (auto) 0.5f else 1f),
        )
        FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            FilterChip(selected = auto, enabled = enabled, onClick = { onChange(102) }, label = { Text(stringResource(R.string.auto)) })
            FAN_PRESETS.forEach { (v, l) ->
                FilterChip(selected = value == v, enabled = enabled, onClick = { onChange(v) }, label = { Text(stringResource(l)) })
            }
        }
    }
}

@Composable
private fun TimersSection(
    t: Timers,
    scenes: List<Scene>,
    enabled: Boolean,
    onSet: (Timers) -> Unit,
    deviceOn: Int?,
    deviceOff: Int?,
    onDevice: (Control) -> Unit,
) {
    var hour by remember { mutableFloatStateOf(7f) }
    var offIn by remember { mutableFloatStateOf(60f) }
    var scene by remember { mutableStateOf<Scene?>(null) }
    Section(stringResource(R.string.timers)) {
        Text(stringResource(R.string.timers_hint), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        if (t.onAt != null) {
            LabeledRow(stringResource(R.string.switch_on_at), clock(t.onAt)) {
                TextButton(onClick = { onSet(t.copy(onAt = null, onSceneId = "")) }) { Text(stringResource(R.string.delete)) }
            }
        } else {
            LabeledRow(stringResource(R.string.switch_on_at), "%02d:%02d".format(hourOf(hour), minuteOf(hour)) + (scene?.let { " · " + it.name } ?: "")) {
                TextButton(enabled = enabled, onClick = {
                    val at = LocalDate.now().atTime(LocalTime.of(hourOf(hour), minuteOf(hour))).atZone(ZoneId.systemDefault()).toInstant()
                    val next = if (at.isBefore(Instant.now())) at.plusSeconds(86400) else at
                    onSet(t.copy(onAt = next.toString(), onSceneId = scene?.id ?: ""))
                }) { Text("OK") }
            }
            Slider(value = hour, onValueChange = { hour = it }, valueRange = 0f..23.75f, steps = 94, enabled = enabled)
            if (scenes.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    FilterChip(selected = scene == null, onClick = { scene = null }, label = { Text(stringResource(R.string.on)) })
                    scenes.take(3).forEach { sc -> FilterChip(selected = scene?.id == sc.id, onClick = { scene = sc }, label = { Text(sc.name, maxLines = 1) }) }
                }
            }
        }
        if (t.offAt != null) {
            LabeledRow(stringResource(R.string.switch_off_in), clock(t.offAt)) {
                TextButton(onClick = { onSet(t.copy(offAt = null)) }) { Text(stringResource(R.string.delete)) }
            }
        } else {
            val m = offIn.roundToInt()
            LabeledRow(stringResource(R.string.switch_off_in), if (m < 60) "$m min" else "${m / 60} h ${m % 60} min") {
                TextButton(enabled = enabled, onClick = { onSet(t.copy(offAt = Instant.now().plusSeconds(m * 60L).toString())) }) { Text("OK") }
            }
            Slider(value = offIn, onValueChange = { offIn = it }, valueRange = 15f..720f, steps = 46, enabled = enabled)
        }
        Text(stringResource(R.string.device_timers), fontWeight = FontWeight.Medium, fontSize = 13.sp)
        LabeledRow(stringResource(R.string.device_timer_off), deviceOff?.let { "$it min" }) {
            if (deviceOff != null) {
                TextButton(enabled = enabled, onClick = { onDevice(Control(offTimer = 0)) }) { Text(stringResource(R.string.delete)) }
            } else {
                Row {
                    listOf(60, 120, 240).forEach { v ->
                        TextButton(enabled = enabled, onClick = { onDevice(Control(offTimer = v)) }) { Text("${v / 60} h") }
                    }
                }
            }
        }
        LabeledRow(stringResource(R.string.device_timer_on), deviceOn?.let { "$it min" }) {
            if (deviceOn != null) {
                TextButton(enabled = enabled, onClick = { onDevice(Control(onTimer = 0)) }) { Text(stringResource(R.string.delete)) }
            } else {
                Row {
                    listOf(60, 240, 480).forEach { v ->
                        TextButton(enabled = enabled, onClick = { onDevice(Control(onTimer = v)) }) { Text("${v / 60} h") }
                    }
                }
            }
        }
    }
}

private fun hourOf(v: Float): Int = (v * 4).roundToInt() / 4
private fun minuteOf(v: Float): Int = ((v * 4).roundToInt() % 4) * 15
