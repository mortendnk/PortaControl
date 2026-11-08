package io.github.mortendnk.portacontrol.data

import io.github.mortendnk.portacontrol.PortaControlApp

/**
 * Access for the widget and the quick settings tile, which run without the
 * live stream: they read the device once and send commands directly.
 */
object Favorite {
    suspend fun api(app: PortaControlApp): Pair<Api, String?>? {
        val p = app.settings.current()
        if (!p.configured) return null
        val api = Api(p.serverUrl, p.apiKey)
        return api to p.favorite.ifBlank { null }
    }

    /** The favorite device (or the only/first one). */
    suspend fun device(app: PortaControlApp): Pair<Api, Device>? {
        val (api, fav) = api(app) ?: return null
        val list = runCatching { api.devices() }.getOrNull() ?: return null
        val d = list.firstOrNull { it.id == fav } ?: list.minByOrNull { it.order } ?: return null
        return api to d
    }
}
