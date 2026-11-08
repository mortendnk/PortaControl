package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Thermostat
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.Action
import io.github.mortendnk.portacontrol.data.Repository
import io.github.mortendnk.portacontrol.data.Rule
import io.github.mortendnk.portacontrol.data.Scene
import io.github.mortendnk.portacontrol.data.Schedule
import kotlinx.coroutines.launch

private val DAY_NAMES = mapOf(1 to "Mo", 2 to "Di", 3 to "Mi", 4 to "Do", 5 to "Fr", 6 to "Sa", 0 to "So")

private fun days(d: List<Int>?): String = when {
    d.isNullOrEmpty() || d.size == 7 -> "täglich"
    d.size == 5 && listOf(1, 2, 3, 4, 5).all { it in d } -> "Mo–Fr"
    d.size == 2 && 0 in d && 6 in d -> "Sa, So"
    else -> listOf(1, 2, 3, 4, 5, 6, 0).filter { it in d }.joinToString(", ") { DAY_NAMES[it].orEmpty() }
}

private fun describe(a: Action, scenes: List<Scene>): String = when (a.kind) {
    "on" -> "Einschalten"
    "off" -> "Ausschalten"
    "scene" -> "Szene: " + (scenes.firstOrNull { it.id == a.sceneId }?.name ?: "?")
    "control" -> listOfNotNull(a.control?.mode, a.control?.target?.let { "${it.toInt()} °C" }).joinToString(" · ").ifEmpty { "Einstellen" }
    "notify" -> "Nachricht: " + a.text.orEmpty()
    else -> a.kind
}

@Composable
fun AutomationScreen(repo: Repository, snack: SnackbarHostState) {
    val changes by repo.changes.collectAsState()
    val devices by repo.devices.collectAsState()
    val scope = rememberCoroutineScope()
    var scenes by remember { mutableStateOf<List<Scene>>(emptyList()) }
    var schedules by remember { mutableStateOf<List<Schedule>>(emptyList()) }
    var rules by remember { mutableStateOf<List<Rule>>(emptyList()) }

    suspend fun load() {
        val a = repo.api ?: return
        scenes = runCatching { a.scenes() }.getOrDefault(emptyList())
        schedules = runCatching { a.schedules() }.getOrDefault(emptyList())
        rules = runCatching { a.rules() }.getOrDefault(emptyList())
    }
    LaunchedEffect(changes) { load() }

    LazyColumn(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Section(stringResource(R.string.schedules)) {
                if (schedules.isEmpty()) Text(stringResource(R.string.no_schedules), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 13.sp)
                schedules.forEach { s ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(s.time, fontSize = 22.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.width(72.dp))
                        Column(Modifier.weight(1f)) {
                            Text(s.name, fontWeight = FontWeight.Medium)
                            Text("${days(s.days)} · ${describe(s.action, scenes)}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            if (s.enabled && s.nextRun != null) {
                                Text(stringResource(R.string.next_run) + ": " + clock(s.nextRun), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                        Switch(checked = s.enabled, onCheckedChange = { on ->
                            scope.launch {
                                runCatching { repo.api?.saveSchedule(s.copy(enabled = on)) }.onFailure { snack.showSnackbar(it.message ?: "") }
                                load()
                            }
                        })
                    }
                }
                Text(stringResource(R.string.edit_in_web), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        item {
            Section(stringResource(R.string.scenes)) {
                if (scenes.isEmpty()) Text(stringResource(R.string.no_scenes), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 13.sp)
                scenes.forEach { sc ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Filled.AutoAwesome, null, tint = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.size(10.dp))
                        Text(sc.name, Modifier.weight(1f), fontWeight = FontWeight.Medium)
                        IconButton(onClick = {
                            scope.launch {
                                runCatching { repo.api?.applyScene(sc.id) }
                                    .onSuccess { snack.showSnackbar(sc.name + " ✓") }
                                    .onFailure { snack.showSnackbar(it.message ?: "") }
                            }
                        }) { Icon(Icons.Filled.PlayArrow, stringResource(R.string.apply)) }
                    }
                }
            }
        }
        item {
            Section(stringResource(R.string.rules)) {
                if (rules.isEmpty()) Text(stringResource(R.string.no_rules), color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 13.sp)
                rules.forEach { r ->
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Filled.Thermostat, null, tint = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.size(10.dp))
                        Column(Modifier.weight(1f)) {
                            Text(r.name, fontWeight = FontWeight.Medium)
                            val sensor = when (r.sensor) {
                                "indoor" -> "innen"
                                "outdoor" -> "außen"
                                else -> "Leistung"
                            }
                            val unit = if (r.sensor == "power_watts") "W" else "°C"
                            Text(
                                "${devices[r.device]?.name ?: r.device}: $sensor ${if (r.op == "above") ">" else "<"} ${num(r.value, 1)} $unit → ${describe(r.action, scenes)}",
                                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Switch(checked = r.enabled, onCheckedChange = { on ->
                            scope.launch {
                                runCatching { repo.api?.saveRule(r.copy(enabled = on)) }.onFailure { snack.showSnackbar(it.message ?: "") }
                                load()
                            }
                        })
                    }
                }
            }
        }
    }
}
