package io.github.mortendnk.portacontrol

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.BarChart
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.navigation.NavType
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import androidx.navigation.navArgument
import io.github.mortendnk.portacontrol.data.Api
import io.github.mortendnk.portacontrol.data.Prefs
import io.github.mortendnk.portacontrol.ui.AutomationScreen
import io.github.mortendnk.portacontrol.ui.DeviceScreen
import io.github.mortendnk.portacontrol.ui.DevicesScreen
import io.github.mortendnk.portacontrol.ui.HistoryScreen
import io.github.mortendnk.portacontrol.ui.OnboardingScreen
import io.github.mortendnk.portacontrol.ui.PortaTheme
import io.github.mortendnk.portacontrol.ui.SettingsScreen
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    /** portacontrol://connect?url=…&key=… opened from a QR code or a link. */
    private val pendingLink = mutableStateOf<Pair<String, String>?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        handleIntent(intent)

        val app = application as PortaControlApp
        setContent {
            val prefs by app.settings.prefs.collectAsState(initial = null)
            PortaTheme(prefs?.theme ?: "system") {
                val p = prefs
                if (p != null) Root(app, p, pendingLink.value) { pendingLink.value = null }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        val data = intent?.dataString ?: return
        Api.parseLink(data)?.let { pendingLink.value = it }
    }

    @Composable
    private fun Root(app: PortaControlApp, prefs: Prefs, link: Pair<String, String>?, consumed: () -> Unit) {
        val scope = rememberCoroutineScope()
        if (link != null) {
            LaunchedEffect(link) {
                app.settings.setServer(Api.normalize(link.first), link.second)
                consumed()
            }
        }
        if (!prefs.configured) {
            OnboardingScreen(onConnected = { url, key -> scope.launch { app.settings.setServer(url, key) } })
            return
        }

        val nav = rememberNavController()
        val snack = remember { SnackbarHostState() }
        val backStack by nav.currentBackStackEntryAsState()
        val route = backStack?.destination?.route ?: "devices"

        LaunchedEffect(Unit) {
            app.repo.notices.collect { n ->
                // Push while the app is closed: ntfy (Einstellungen > Benachrichtigungen in the web UI).
                snack.showSnackbar(n.title + (n.text?.let { " – $it" } ?: ""))
            }
        }

        val tabs = listOf(
            Triple("devices", R.string.tab_devices, Icons.Filled.Home),
            Triple("automation", R.string.tab_automation, Icons.Filled.CalendarMonth),
            Triple("history", R.string.tab_history, Icons.Filled.BarChart),
            Triple("settings", R.string.tab_settings, Icons.Filled.Settings),
        )

        Scaffold(
            snackbarHost = { SnackbarHost(snack) },
            bottomBar = {
                NavigationBar {
                    tabs.forEach { (r, label, icon) ->
                        NavigationBarItem(
                            selected = route == r || (r == "devices" && route.startsWith("device/")),
                            onClick = {
                                nav.navigate(r) {
                                    popUpTo("devices") { saveState = true }
                                    launchSingleTop = true
                                    restoreState = true
                                }
                            },
                            icon = { Icon(icon, contentDescription = null) },
                            label = { Text(stringResource(label)) },
                        )
                    }
                }
            },
        ) { pad ->
            NavHost(nav, startDestination = "devices", modifier = Modifier.padding(pad)) {
                composable("devices") { DevicesScreen(app.repo, onOpen = { nav.navigate("device/$it") }) }
                composable("device/{id}", arguments = listOf(navArgument("id") { type = NavType.StringType })) { e ->
                    DeviceScreen(app.repo, e.arguments?.getString("id").orEmpty(), onBack = { nav.popBackStack() }, snack = snack)
                }
                composable("automation") { AutomationScreen(app.repo, snack) }
                composable("history") { HistoryScreen(app.repo) }
                composable("settings") { SettingsScreen(app, prefs) }
            }
        }
    }
}
