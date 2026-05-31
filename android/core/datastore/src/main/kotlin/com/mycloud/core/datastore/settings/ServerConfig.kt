package com.mycloud.core.datastore.settings

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject
import javax.inject.Singleton

private val Context.serverDataStore by preferencesDataStore(name = "server_config")

/**
 * Stores the user-chosen backend base URL. Exposes a synchronous [current] read
 * (cached) for the OkHttp host-selection interceptor and Coil URL builders, plus
 * a suspend [setUrl] that normalizes and persists it.
 */
@Singleton
class ServerConfig @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val key = stringPreferencesKey("base_url")

    @Volatile
    private var cached: String? = null
    private val loaded = AtomicBoolean(false)

    /** The configured base URL (with trailing slash), or the dev default if unset. */
    fun current(): String = read() ?: DEFAULT

    /** Whether the user has explicitly set a server address. */
    fun isConfigured(): Boolean = read() != null

    suspend fun setUrl(url: String) {
        val normalized = normalize(url)
        cached = normalized
        loaded.set(true)
        context.serverDataStore.edit { it[key] = normalized }
    }

    suspend fun load() {
        cached = context.serverDataStore.data.first()[key]
        loaded.set(true)
    }

    private fun read(): String? {
        if (loaded.compareAndSet(false, true)) {
            cached = runBlocking {
                runCatching { context.serverDataStore.data.first()[key] }.getOrNull()
            }
        }
        return cached
    }

    private fun normalize(raw: String): String {
        var s = raw.trim()
        if (s.isEmpty()) return DEFAULT
        if (!s.startsWith("http://") && !s.startsWith("https://")) s = "http://$s"
        if (!s.endsWith("/")) s = "$s/"
        return s
    }

    companion object {
        /** Emulator host-loopback default — the repo's docker-compose serves the
         *  app through nginx (HTTPS, self-signed) on 443. `10.0.2.2` = host localhost. */
        const val DEFAULT = "https://10.0.2.2/"
    }
}
