package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.systemBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AcUnit
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material.icons.filled.Router
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.Api
import io.github.mortendnk.portacontrol.data.FoundServer
import io.github.mortendnk.portacontrol.data.discoverServers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import androidx.activity.compose.rememberLauncherForActivityResult

@Composable
fun OnboardingScreen(onConnected: (String, String) -> Unit) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var url by remember { mutableStateOf("") }
    var key by remember { mutableStateOf("") }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    val found = remember { mutableStateListOf<FoundServer>() }

    LaunchedEffect(Unit) {
        withTimeoutOrNull(20_000) {
            discoverServers(ctx).collect { s -> if (found.none { it.url == s.url }) found.add(s) }
        }
    }

    suspend fun tryConnect(u: String, k: String) {
        busy = true
        error = null
        try {
            val api = Api(u, k)
            api.health()
            val me = api.me()
            if (me.role != "admin") {
                error = ctx.getString(R.string.onboarding_wrong_key)
            } else {
                onConnected(api.base, k)
            }
        } catch (e: Exception) {
            error = e.message ?: e.toString()
        } finally {
            busy = false
        }
    }

    val scanner = rememberLauncherForActivityResult(ScanContract()) { res ->
        val text = res.contents ?: return@rememberLauncherForActivityResult
        val parsed = Api.parseLink(text)
        if (parsed == null) {
            error = ctx.getString(R.string.onboarding_bad_qr)
        } else {
            url = parsed.first
            key = parsed.second
            scope.launch { tryConnect(Api.normalize(parsed.first), parsed.second) }
        }
    }

    Column(
        Modifier
            .fillMaxSize()
            .background(MaterialTheme.colorScheme.background)
            .systemBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(
                Modifier.size(44.dp).background(MaterialTheme.colorScheme.primary, RoundedCornerShape(14.dp)),
                contentAlignment = Alignment.Center,
            ) { Icon(Icons.Filled.AcUnit, null, tint = MaterialTheme.colorScheme.onPrimary) }
            Spacer(Modifier.size(12.dp))
            Text("PortaControl", fontSize = 24.sp, fontWeight = FontWeight.SemiBold)
        }
        Text(stringResource(R.string.onboarding_intro), color = MaterialTheme.colorScheme.onSurfaceVariant)

        Button(
            onClick = {
                scanner.launch(
                    ScanOptions().setDesiredBarcodeFormats(ScanOptions.QR_CODE).setBeepEnabled(false)
                        .setOrientationLocked(false).setPrompt(ctx.getString(R.string.onboarding_scan_prompt)),
                )
            },
            modifier = Modifier.fillMaxWidth().height(52.dp),
        ) {
            Icon(Icons.Filled.QrCodeScanner, null)
            Spacer(Modifier.size(8.dp))
            Text(stringResource(R.string.onboarding_scan))
        }

        if (found.isNotEmpty()) {
            Section(stringResource(R.string.onboarding_found)) {
                found.forEach { s ->
                    Row(
                        Modifier.fillMaxWidth().clickable { url = s.url }.padding(vertical = 6.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Icon(Icons.Filled.Router, null, tint = MaterialTheme.colorScheme.primary)
                        Spacer(Modifier.size(10.dp))
                        Column {
                            Text(s.name, fontWeight = FontWeight.Medium)
                            Text(s.url, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }

        Section(stringResource(R.string.onboarding_manual)) {
            OutlinedTextField(
                url, { url = it }, Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.server_address)) },
                placeholder = { Text("192.168.178.20:8080") }, singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            )
            OutlinedTextField(
                key, { key = it.trim() }, Modifier.fillMaxWidth(), label = { Text(stringResource(R.string.api_key)) },
                singleLine = true, visualTransformation = PasswordVisualTransformation(),
            )
            Text(stringResource(R.string.onboarding_key_hint), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            OutlinedButton(
                onClick = { scope.launch { tryConnect(Api.normalize(url), key) } },
                enabled = !busy && url.isNotBlank() && key.isNotBlank(),
                modifier = Modifier.fillMaxWidth(),
            ) {
                if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) else Text(stringResource(R.string.connect))
            }
        }
        error?.let { Text(it, color = MaterialTheme.colorScheme.error) }
    }
}
