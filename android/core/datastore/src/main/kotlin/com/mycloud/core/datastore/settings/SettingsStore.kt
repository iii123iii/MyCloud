package com.mycloud.core.datastore.settings

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

enum class ThemeMode { SYSTEM, LIGHT, DARK }

private val Context.settingsDataStore by preferencesDataStore(name = "settings")

/** Non-secret user preferences. */
@Singleton
class SettingsStore @Inject constructor(
    @ApplicationContext private val context: Context,
) {
    private val ds = context.settingsDataStore

    val themeMode: Flow<ThemeMode> = ds.data.map { prefs ->
        runCatching { ThemeMode.valueOf(prefs[THEME_KEY] ?: ThemeMode.SYSTEM.name) }
            .getOrDefault(ThemeMode.SYSTEM)
    }

    val appLockEnabled: Flow<Boolean> = ds.data.map { it[APP_LOCK_KEY] ?: false }

    /** The user whose data currently populates the local cache (null = none).
     *  Used to detect a user-switch at login so the previous user's cache is wiped. */
    val cachedUserId: Flow<String?> = ds.data.map { it[CACHED_USER_ID_KEY] }

    suspend fun setThemeMode(mode: ThemeMode) {
        ds.edit { it[THEME_KEY] = mode.name }
    }

    suspend fun setAppLockEnabled(enabled: Boolean) {
        ds.edit { it[APP_LOCK_KEY] = enabled }
    }

    suspend fun setCachedUserId(userId: String?) {
        ds.edit {
            if (userId == null) it.remove(CACHED_USER_ID_KEY) else it[CACHED_USER_ID_KEY] = userId
        }
    }

    private companion object {
        val THEME_KEY = stringPreferencesKey("theme_mode")
        val APP_LOCK_KEY = booleanPreferencesKey("app_lock_enabled")
        val CACHED_USER_ID_KEY = stringPreferencesKey("cached_user_id")
    }
}
