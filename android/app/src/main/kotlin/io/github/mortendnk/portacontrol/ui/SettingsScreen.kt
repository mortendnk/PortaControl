package io.github.mortendnk.portacontrol.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.mortendnk.portacontrol.BuildConfig
import io.github.mortendnk.portacontrol.PortaControlApp
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.Conn
import io.github.mortendnk.portacontrol.data.LogEntry
import io.github.mortendnk.portacontrol.data.Prefs
import io.github.mortendnk.portacontrol.widget.ClimateWidget
import kotlinx.coroutines.launch

@Composable
fun SettingsScreen(app: PortaControlApp, prefs: Prefs) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    val devices by app.repo.devices.collectAsState()
    val conn by app.repo.conn.collectAsState()
    var version by remember { mutableStateOf("") }
    var log by remember { mutableStateOf<List<LogEntry>>(emptyList()) }

    LaunchedEffect(prefs.serverUrl) {
        version = runCatching { app.repo.api?.health()?.version }.getOrNull().orEmpty()
        log = runCatching { app.repo.api?.log() }.getOrNull().orEmpty()
    }

    Column(
        Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background).verticalScroll(rememberScrollState()).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Section(stringResource(R.string.server)) {
            LabeledRow(prefs.serverUrl, stringResource(if (conn == Conn.Live) R.string.connected else R.string.connecting) + if (version.isNotBlank()) " · $version" else "") {}
            OutlinedButton(
                onClick = { ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(prefs.serverUrl))) },
                modifier = Modifier.fillMaxWidth(),
            ) { Text(stringResource(R.string.open_web)) }
            Text(stringResource(R.string.web_hint), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }

        Section(stringResource(R.string.favorite)) {
            Text(stringResource(R.string.favorite_hint), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            devices.values.sortedBy { it.order }.forEach { d ->
                LabeledRow(d.name, d.room.ifBlank { null }) {
                    RadioButton(selected = prefs.favorite == d.id || (prefs.favorite.isBlank() && devices.size == 1), onClick = {
                        scope.launch {
                            app.settings.setFavorite(d.id)
                            ClimateWidget.updateAll(ctx)
                        }
                    })
                }
            }
        }

        Section(stringResource(R.string.theme)) {
            listOf("system" to R.string.theme_system, "light" to R.string.theme_light, "dark" to R.string.theme_dark).forEach { (v, l) ->
                LabeledRow(stringResource(l)) {
                    RadioButton(selected = prefs.theme == v, onClick = { scope.launch { app.settings.setTheme(v) } })
                }
            }
        }

        if (log.isNotEmpty()) {
            Section(stringResource(R.string.log)) {
                log.take(30).forEach { e ->
                    Text(
                        clock(e.time) + "  " + e.text,
                        fontSize = 12.sp,
                        color = when (e.level) {
                            "error" -> MaterialTheme.colorScheme.error
                            "warn" -> Pc.warn
                            else -> MaterialTheme.colorScheme.onSurface
                        },
                    )
                }
            }
        }

        Section {
            Button(
                onClick = { scope.launch { app.settings.clearServer() } },
                modifier = Modifier.fillMaxWidth(),
            ) { Text(stringResource(R.string.disconnect)) }
            Text("PortaControl ${BuildConfig.VERSION_NAME}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
