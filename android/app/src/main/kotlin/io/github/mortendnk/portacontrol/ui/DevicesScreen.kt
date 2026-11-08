package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.PowerSettingsNew
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AssistChip
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledIconToggleButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.Conn
import io.github.mortendnk.portacontrol.data.Control
import io.github.mortendnk.portacontrol.data.Device
import io.github.mortendnk.portacontrol.data.Repository
import io.github.mortendnk.portacontrol.data.Scene
import kotlinx.coroutines.launch

@Composable
fun DevicesScreen(repo: Repository, onOpen: (String) -> Unit) {
    val devices by repo.devices.collectAsState()
    val conn by repo.conn.collectAsState()
    val changes by repo.changes.collectAsState()
    val scope = rememberCoroutineScope()
    var scenes by remember { mutableStateOf<List<Scene>>(emptyList()) }

    LaunchedEffect(changes, repo.api) {
        scenes = runCatching { repo.api?.scenes() }.getOrNull().orEmpty()
    }

    val list = devices.values.sortedWith(compareBy({ it.order }, { it.name }))
    val watts = list.sumOf { if (it.state?.power == true) it.state.watts ?: 0.0 else 0.0 }
    val kwh = list.sumOf { it.today?.kwh ?: 0.0 }
    val cost = list.sumOf { it.today?.cost ?: 0.0 }

    LazyColumn(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("PortaControl", fontSize = 22.sp, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                if (conn == Conn.Connecting || conn == Conn.Error) {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                }
            }
        }
        if (list.isNotEmpty()) {
            item {
                Section {
                    Row {
                        StatBox(stringResource(R.string.power), "${num(watts, 0)} W", Modifier.weight(1f))
                        StatBox(stringResource(R.string.today), "${num(kwh, 2)} kWh", Modifier.weight(1f))
                        StatBox(stringResource(R.string.cost), money(cost, list.firstOrNull()?.today?.currency ?: "EUR"), Modifier.weight(1f))
                    }
                }
            }
        }
        if (scenes.isNotEmpty()) {
            item {
                LazyRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(scenes, key = { it.id }) { s ->
                        AssistChip(
                            onClick = { scope.launch { runCatching { repo.api?.applyScene(s.id) } } },
                            label = { Text(s.name) },
                            leadingIcon = { Icon(Icons.Filled.AutoAwesome, null, Modifier.size(18.dp)) },
                        )
                    }
                }
            }
        }
        if (list.isEmpty()) {
            item {
                Section {
                    Text(
                        if (conn == Conn.Live) stringResource(R.string.no_devices) else stringResource(R.string.connecting),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        items(list, key = { it.id }) { d ->
            DeviceCard(d, onOpen = { onOpen(d.id) }, onPower = { on -> scope.launch { repo.control(d.id, Control(power = on)) } })
        }
    }
}

@Composable
private fun DeviceCard(d: Device, onOpen: () -> Unit, onPower: (Boolean) -> Unit) {
    val s = d.state
    val on = s?.power == true
    val color = if (on) modeColor(s?.mode) else MaterialTheme.colorScheme.surfaceVariant
    Section(modifier = Modifier.clickable { onOpen() }) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(44.dp).background(color, RoundedCornerShape(14.dp)), contentAlignment = Alignment.Center) {
                Icon(modeIcon(s?.mode), null, tint = if (on) Color.White else MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Spacer(Modifier.size(12.dp))
            Column(Modifier.weight(1f)) {
                Text(d.name, fontWeight = FontWeight.SemiBold, maxLines = 1)
                val sub = when {
                    !d.online -> stringResource(R.string.unreachable)
                    on -> stringResource(MODES.first { it.id == s?.mode }.label) + " · " + num(s?.target, 0) + " °C"
                    else -> stringResource(R.string.off)
                }
                Text(
                    listOf(d.room, sub).filter { it.isNotBlank() }.joinToString(" · "),
                    fontSize = 12.sp,
                    color = if (!d.online) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            FilledIconToggleButton(checked = on, onCheckedChange = onPower, enabled = s != null && !d.locked, shape = CircleShape) {
                Icon(Icons.Filled.PowerSettingsNew, stringResource(R.string.power))
            }
        }
        Row {
            StatBox(stringResource(R.string.indoor), temp(s?.indoor), Modifier.weight(1f))
            StatBox(stringResource(R.string.outdoor), temp(s?.outdoor), Modifier.weight(1f))
            StatBox(stringResource(R.string.power), s?.watts?.let { num(it, 0) + " W" } ?: "–", Modifier.weight(1f))
        }
        if (s?.filterAlert == true || (s?.errorCode ?: 0) != 0) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Filled.Warning, null, tint = Pc.warn, modifier = Modifier.size(16.dp))
                Spacer(Modifier.size(6.dp))
                Text(
                    if ((s?.errorCode ?: 0) != 0) stringResource(R.string.error_code, s!!.errorCode) else stringResource(R.string.filter_alert),
                    fontSize = 12.sp,
                    color = Pc.warn,
                )
            }
        }
        d.today?.let {
            Text(
                stringResource(R.string.today) + ": " + num(it.kwh, 2) + " kWh · " + money(it.cost, it.currency),
                fontSize = 12.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.fillMaxWidth().padding(top = 2.dp),
            )
        }
    }
}
