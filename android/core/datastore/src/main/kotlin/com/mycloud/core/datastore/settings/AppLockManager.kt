package com.mycloud.core.datastore.settings

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Tracks whether the biometric app-lock is currently engaged. The actual
 * `BiometricPrompt` is shown by the feature layer; this just holds the gate
 * state and exposes whether the user enabled the lock.
 */
@Singleton
class AppLockManager @Inject constructor(
    settingsStore: SettingsStore,
) {
    private val _locked = MutableStateFlow(false)
    val locked: StateFlow<Boolean> = _locked.asStateFlow()

    /** Whether the user turned the app-lock on in Settings. */
    val enabled: Flow<Boolean> = settingsStore.appLockEnabled

    /** Engage the lock (e.g. on cold start or after a background timeout). */
    fun lock() {
        _locked.value = true
    }

    /** Clear the lock after a successful biometric/credential unlock. */
    fun unlock() {
        _locked.value = false
    }
}
