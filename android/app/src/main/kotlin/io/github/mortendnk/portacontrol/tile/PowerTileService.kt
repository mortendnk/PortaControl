package io.github.mortendnk.portacontrol.tile

import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import io.github.mortendnk.portacontrol.PortaControlApp
import io.github.mortendnk.portacontrol.R
import io.github.mortendnk.portacontrol.data.Control
import io.github.mortendnk.portacontrol.data.Favorite
import io.github.mortendnk.portacontrol.widget.ClimateWidget
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Quick settings tile: switches the favorite air conditioner on and off. */
class PowerTileService : TileService() {
    private var scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onStartListening() {
        super.onStartListening()
        scope.launch { refresh() }
    }

    override fun onClick() {
        super.onClick()
        val app = application as PortaControlApp
        scope.launch {
            val (api, d) = Favorite.device(app) ?: return@launch
            val on = d.state?.power != true
            withContext(Dispatchers.Main) { show(on, d.name, busy = true) }
            runCatching { api.control(d.id, Control(power = on)) }
            refresh()
            ClimateWidget.updateAll(app)
        }
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    private suspend fun refresh() {
        val app = application as PortaControlApp
        val fav = Favorite.device(app)
        withContext(Dispatchers.Main) {
            if (fav == null) {
                qsTile?.apply {
                    state = Tile.STATE_UNAVAILABLE
                    label = getString(R.string.app_name)
                    updateTile()
                }
            } else {
                val d = fav.second
                val t = d.state?.indoor?.let { " · %.1f °C".format(it) }.orEmpty()
                show(d.state?.power == true, d.name + t, busy = false)
            }
        }
    }

    private fun show(on: Boolean, label: String, busy: Boolean) {
        qsTile?.apply {
            state = if (on) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
            this.label = label
            if (android.os.Build.VERSION.SDK_INT >= 29) {
                subtitle = getString(if (busy) R.string.sending else if (on) R.string.on else R.string.off)
            }
            updateTile()
        }
    }
}
