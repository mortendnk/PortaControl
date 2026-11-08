package io.github.mortendnk.portacontrol.widget

import android.content.Context
import androidx.compose.runtime.Composable
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.glance.GlanceId
import androidx.glance.GlanceModifier
import androidx.glance.GlanceTheme
import androidx.glance.action.ActionParameters
import androidx.glance.action.actionParametersOf
import androidx.glance.action.clickable
import androidx.glance.appwidget.GlanceAppWidget
import androidx.glance.appwidget.GlanceAppWidgetManager
import androidx.glance.appwidget.GlanceAppWidgetReceiver
import androidx.glance.appwidget.action.ActionCallback
import androidx.glance.appwidget.action.actionRunCallback
import androidx.glance.action.actionStartActivity
import androidx.glance.appwidget.cornerRadius
import androidx.glance.appwidget.provideContent
import androidx.glance.appwidget.updateAll
import androidx.glance.background
import androidx.glance.layout.Alignment
import androidx.glance.layout.Box
import androidx.glance.layout.Column
import androidx.glance.layout.Row
import androidx.glance.layout.Spacer
import androidx.glance.layout.fillMaxSize
import androidx.glance.layout.fillMaxWidth
import androidx.glance.layout.padding
import androidx.glance.layout.size
import androidx.glance.layout.width
import androidx.glance.text.FontWeight
import androidx.glance.text.Text
import androidx.glance.text.TextStyle
import io.github.mortendnk.portacontrol.MainActivity
import io.github.mortendnk.portacontrol.PortaControlApp
import io.github.mortendnk.portacontrol.data.Control
import io.github.mortendnk.portacontrol.data.Device
import io.github.mortendnk.portacontrol.data.Favorite
import java.util.Locale

/** Home screen widget: favorite device with room temperature, target −/+ and power. */
class ClimateWidget : GlanceAppWidget() {
    override suspend fun provideGlance(context: Context, id: GlanceId) {
        val app = context.applicationContext as PortaControlApp
        val d = Favorite.device(app)?.second
        provideContent { GlanceTheme { Content(d) } }
    }

    @Composable
    private fun Content(d: Device?) {
        val s = d?.state
        Column(
            GlanceModifier.fillMaxSize().background(GlanceTheme.colors.widgetBackground).cornerRadius(20.dp).padding(12.dp)
                .clickable(actionStartActivity<MainActivity>()),
        ) {
            if (d == null) {
                Text("PortaControl", style = TextStyle(fontWeight = FontWeight.Bold, color = GlanceTheme.colors.onSurface))
                Text("Nicht verbunden", style = TextStyle(fontSize = 12.sp, color = GlanceTheme.colors.onSurfaceVariant))
                return@Column
            }
            Text(d.name, maxLines = 1, style = TextStyle(fontWeight = FontWeight.Bold, color = GlanceTheme.colors.onSurface))
            Text(
                (s?.indoor?.let { String.format(Locale.getDefault(), "%.1f °C", it) } ?: "–") +
                    if (s?.power == true) " → ${s.target.toInt()} °C" else " · aus",
                style = TextStyle(fontSize = 13.sp, color = GlanceTheme.colors.onSurfaceVariant),
            )
            Spacer(GlanceModifier.size(8.dp))
            Row(GlanceModifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Pill("−", actionRunCallback<StepAction>(actionParametersOf(StepAction.delta to -1)))
                Spacer(GlanceModifier.width(6.dp))
                Pill("+", actionRunCallback<StepAction>(actionParametersOf(StepAction.delta to 1)))
                Spacer(GlanceModifier.defaultWeight())
                Pill(if (s?.power == true) "Aus" else "Ein", actionRunCallback<PowerAction>(), highlight = s?.power == true)
            }
        }
    }

    @Composable
    private fun Pill(text: String, action: androidx.glance.action.Action, highlight: Boolean = false) {
        Box(
            GlanceModifier
                .background(if (highlight) GlanceTheme.colors.primary else GlanceTheme.colors.secondaryContainer)
                .cornerRadius(16.dp)
                .padding(horizontal = 14.dp, vertical = 6.dp)
                .clickable(action),
            contentAlignment = Alignment.Center,
        ) {
            Text(
                text,
                style = TextStyle(
                    fontWeight = FontWeight.Bold,
                    color = if (highlight) GlanceTheme.colors.onPrimary else GlanceTheme.colors.onSecondaryContainer,
                ),
            )
        }
    }

    companion object {
        suspend fun updateAll(context: Context) {
            runCatching {
                if (GlanceAppWidgetManager(context).getGlanceIds(ClimateWidget::class.java).isNotEmpty()) {
                    ClimateWidget().updateAll(context)
                }
            }
        }
    }
}

class PowerAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        val app = context.applicationContext as PortaControlApp
        val (api, d) = Favorite.device(app) ?: return
        runCatching { api.control(d.id, Control(power = d.state?.power != true)) }
        ClimateWidget().updateAll(context)
    }
}

class StepAction : ActionCallback {
    override suspend fun onAction(context: Context, glanceId: GlanceId, parameters: ActionParameters) {
        val app = context.applicationContext as PortaControlApp
        val (api, d) = Favorite.device(app) ?: return
        val step = parameters[delta] ?: return
        val cur = d.state?.target ?: 24.0
        val target = (cur + step).coerceIn(16.0, 30.0)
        runCatching { api.control(d.id, Control(target = target)) }
        ClimateWidget().updateAll(context)
    }

    companion object {
        val delta = ActionParameters.Key<Int>("delta")
    }
}

class ClimateWidgetReceiver : GlanceAppWidgetReceiver() {
    override val glanceAppWidget: GlanceAppWidget = ClimateWidget()
}
