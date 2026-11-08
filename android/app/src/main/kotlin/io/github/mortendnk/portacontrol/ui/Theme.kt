package io.github.mortendnk.portacontrol.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Same tokens as the web UI (server/web/src/styles.css).
object Pc {
    val coolLight = Color(0xFF1F8FD6)
    val heatLight = Color(0xFFE0663A)
    val dryLight = Color(0xFF8A6BD1)
    val fanLight = Color(0xFF2AA58A)
    val autoLight = Color(0xFF6B7C8F)
    val coolDark = Color(0xFF4CB3F2)
    val heatDark = Color(0xFFF08A5F)
    val dryDark = Color(0xFFA990E6)
    val fanDark = Color(0xFF4CC7A7)
    val autoDark = Color(0xFF9AAABB)
    val warn = Color(0xFFB77A14)
    val ok = Color(0xFF2C8A5B)
}

private val Light = lightColorScheme(
    primary = Color(0xFF1F6FB2),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFDBE9F6),
    onPrimaryContainer = Color(0xFF0B2E4B),
    background = Color(0xFFEEF3F8),
    surface = Color(0xFFFFFFFF),
    surfaceVariant = Color(0xFFE3EAF2),
    surfaceContainer = Color(0xFFFFFFFF),
    surfaceContainerHigh = Color(0xFFF3F6FA),
    onSurface = Color(0xFF121A24),
    onSurfaceVariant = Color(0xFF5C6B7C),
    outline = Color(0xFFD3DCE6),
    outlineVariant = Color(0xFFD3DCE6),
    error = Color(0xFFC23B3B),
)

private val Dark = darkColorScheme(
    primary = Color(0xFF5AA9EC),
    onPrimary = Color(0xFF08121C),
    primaryContainer = Color(0xFF1B3043),
    onPrimaryContainer = Color(0xFFD6E8F8),
    background = Color(0xFF0F1318),
    surface = Color(0xFF171D24),
    surfaceVariant = Color(0xFF202833),
    surfaceContainer = Color(0xFF171D24),
    surfaceContainerHigh = Color(0xFF1C232B),
    onSurface = Color(0xFFE6EDF4),
    onSurfaceVariant = Color(0xFF93A2B3),
    outline = Color(0xFF2C3642),
    outlineVariant = Color(0xFF2C3642),
    error = Color(0xFFEC6B6B),
)

@Composable
fun PortaTheme(theme: String = "system", content: @Composable () -> Unit) {
    val dark = when (theme) {
        "dark" -> true
        "light" -> false
        else -> isSystemInDarkTheme()
    }
    MaterialTheme(colorScheme = if (dark) Dark else Light, content = content)
}

@Composable
fun modeColor(mode: String?): Color {
    val dark = MaterialTheme.colorScheme.background == Color(0xFF0F1318)
    return when (mode) {
        "cool" -> if (dark) Pc.coolDark else Pc.coolLight
        "heat" -> if (dark) Pc.heatDark else Pc.heatLight
        "dry" -> if (dark) Pc.dryDark else Pc.dryLight
        "fan" -> if (dark) Pc.fanDark else Pc.fanLight
        else -> if (dark) Pc.autoDark else Pc.autoLight
    }
}
