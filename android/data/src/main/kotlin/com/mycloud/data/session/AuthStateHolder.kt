package com.mycloud.data.session

import com.mycloud.core.model.AuthState
import com.mycloud.core.model.User
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Single source of truth for the session state, shared by the repository (which
 * sets it on login/restore/logout) and the [com.mycloud.data.auth.TokenProviderImpl]
 * (which flips it to LOGGED_OUT when refresh fails / the family is revoked).
 * Decoupling them through this holder avoids a Hilt dependency cycle.
 */
@Singleton
class AuthStateHolder @Inject constructor() {

    private val _state = MutableStateFlow(AuthState.LOADING)
    val state: StateFlow<AuthState> = _state.asStateFlow()

    private val _user = MutableStateFlow<User?>(null)
    val user: StateFlow<User?> = _user.asStateFlow()

    fun setState(state: AuthState) {
        _state.value = state
    }

    fun setUser(user: User?) {
        _user.value = user
    }
}
