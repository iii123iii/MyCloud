package com.mycloud.android

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.datastore.settings.AppLockManager
import com.mycloud.core.model.AuthState
import com.mycloud.data.realtime.RealtimeRepository
import com.mycloud.data.repository.AuthRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import javax.inject.Inject

/**
 * Drives the root navigation gate. Combines the session [AuthState] with the
 * app-lock signal so an authenticated-but-locked session shows the lock screen.
 * Triggers session restore (and engages the lock on cold start) on creation.
 */
@HiltViewModel
class RootViewModel @Inject constructor(
    private val authRepository: AuthRepository,
    private val appLockManager: AppLockManager,
    private val realtimeRepository: RealtimeRepository,
) : ViewModel() {

    val gate: StateFlow<AuthState> = combine(
        authRepository.authState,
        appLockManager.locked,
        appLockManager.enabled,
    ) { state, locked, lockEnabled ->
        if (state == AuthState.AUTHENTICATED && lockEnabled && locked) AuthState.LOCKED else state
    }.stateIn(viewModelScope, SharingStarted.Eagerly, AuthState.LOADING)

    init {
        viewModelScope.launch {
            // Restore first, then engage the lock only if we actually have an
            // authenticated session — avoids a LOCKED flicker for logged-out users.
            authRepository.restoreSession()
            if (authRepository.authState.value == AuthState.AUTHENTICATED && appLockManager.enabled.first()) {
                appLockManager.lock()
            }
        }
        // Hold the realtime socket open only while signed in.
        viewModelScope.launch {
            authRepository.authState.collect { state ->
                if (state == AuthState.AUTHENTICATED) realtimeRepository.start() else realtimeRepository.stop()
            }
        }
    }

    fun logout() {
        viewModelScope.launch { authRepository.logout() }
    }
}
