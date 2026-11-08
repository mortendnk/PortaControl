package io.github.mortendnk.portacontrol.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore(name = "settings")

data class Prefs(
    val serverUrl: String = "",
    val apiKey: String = "",
    val favorite: String = "",
    val theme: String = "system",
) {
    val configured: Boolean get() = serverUrl.isNotBlank() && apiKey.isNotBlank()
}

/** Server address, API key and app preferences (DataStore). */
class Settings(private val context: Context) {
    private val kUrl = stringPreferencesKey("server_url")
    private val kKey = stringPreferencesKey("api_key")
    private val kFav = stringPreferencesKey("favorite_device")
    private val kTheme = stringPreferencesKey("theme")

    val prefs: Flow<Prefs> = context.dataStore.data.map {
        Prefs(it[kUrl].orEmpty(), it[kKey].orEmpty(), it[kFav].orEmpty(), it[kTheme] ?: "system")
    }

    suspend fun current(): Prefs = prefs.first()

    suspend fun setServer(url: String, key: String) {
        context.dataStore.edit {
            it[kUrl] = url
            it[kKey] = key
        }
    }

    suspend fun clearServer() {
        context.dataStore.edit {
            it.remove(kUrl)
            it.remove(kKey)
        }
    }

    suspend fun setFavorite(id: String) {
        context.dataStore.edit { it[kFav] = id }
    }

    suspend fun setTheme(theme: String) {
        context.dataStore.edit { it[kTheme] = theme }
    }
}
