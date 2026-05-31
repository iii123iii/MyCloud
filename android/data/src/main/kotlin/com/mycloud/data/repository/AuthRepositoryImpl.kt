package com.mycloud.data.repository

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.datastore.settings.ServerConfig
import com.mycloud.core.datastore.settings.SettingsStore
import com.mycloud.core.datastore.token.SecureTokenStore
import com.mycloud.core.model.AuthState
import com.mycloud.core.model.User
import com.mycloud.core.network.api.AuthApi
import com.mycloud.core.network.dto.ChangePasswordRequestDto
import com.mycloud.core.network.dto.DeleteAccountRequestDto
import com.mycloud.core.network.dto.LoginRequestDto
import com.mycloud.core.network.dto.LogoutRequestDto
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.data.mapper.toDomain
import com.mycloud.data.mapper.toTokenBundle
import com.mycloud.data.mapper.toUser
import com.mycloud.data.session.AuthStateHolder
import com.mycloud.data.session.SessionCacheCleaner
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.serialization.json.Json
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class AuthRepositoryImpl @Inject constructor(
    private val authApi: AuthApi,
    private val tokenStore: SecureTokenStore,
    private val authStateHolder: AuthStateHolder,
    private val serverConfig: ServerConfig,
    private val settingsStore: SettingsStore,
    private val cacheCleaner: SessionCacheCleaner,
    private val json: Json,
) : AuthRepository {

    override val authState: StateFlow<AuthState> = authStateHolder.state
    override val currentUser: StateFlow<User?> = authStateHolder.user

    override fun serverUrl(): String = serverConfig.current()

    override suspend fun setServerUrl(url: String) = serverConfig.setUrl(url)

    override suspend fun restoreSession() {
        serverConfig.load()
        val bundle = tokenStore.load()
        if (bundle == null) {
            authStateHolder.setState(AuthState.LOGGED_OUT)
            return
        }
        when (val result = safeApiCall(json) { authApi.me() }) {
            is NetworkResult.Success -> applyUser(result.data.toDomain())
            NetworkResult.PasswordChangeRequired ->
                authStateHolder.setState(AuthState.NEEDS_PASSWORD_CHANGE)
            NetworkResult.Unauthorized -> {
                tokenStore.clear()
                wipeCache()
                authStateHolder.setState(AuthState.LOGGED_OUT)
            }
            // Transient/offline failure: we still hold a session, so let the user
            // in; screens surface their own errors and a later call can revalidate.
            else -> authStateHolder.setState(AuthState.AUTHENTICATED)
        }
    }

    override suspend fun login(email: String, password: String): NetworkResult<Unit> {
        val result = safeApiCall(json) { authApi.login(LoginRequestDto(email, password)) }
        if (result is NetworkResult.Success) {
            val tokens = result.data
            // A different user signing in on this device must not inherit the prior
            // user's cached files/thumbnails/previews. (Same user keeps their cache.)
            val previousUserId = settingsStore.cachedUserId.first()
            if (previousUserId != null && previousUserId != tokens.userId) {
                cacheCleaner.clear()
            }
            settingsStore.setCachedUserId(tokens.userId)
            tokenStore.save(tokens.toTokenBundle())
            authStateHolder.setUser(tokens.toUser())
            authStateHolder.setState(
                if (tokens.mustChangePassword) AuthState.NEEDS_PASSWORD_CHANGE else AuthState.AUTHENTICATED,
            )
        }
        return result.map { }
    }

    override suspend fun changePassword(
        oldPassword: String,
        newPassword: String,
    ): NetworkResult<Unit> {
        val result = safeApiCall(json) {
            authApi.changePassword(ChangePasswordRequestDto(oldPassword, newPassword))
        }
        if (result is NetworkResult.Success) {
            // Flag cleared server-side; refresh the profile and unblock the app.
            when (val me = safeApiCall(json) { authApi.me() }) {
                is NetworkResult.Success -> applyUser(me.data.toDomain())
                else -> authStateHolder.setState(AuthState.AUTHENTICATED)
            }
        }
        return result.map { }
    }

    override suspend fun deleteAccount(password: String): NetworkResult<Unit> =
        safeUnitApiCall(json) { authApi.deleteAccount(DeleteAccountRequestDto(password)) }

    override suspend fun logout() {
        val refresh = tokenStore.currentRefreshToken()
        runCatching { authApi.logout(LogoutRequestDto(refresh)) }
        tokenStore.clear()
        wipeCache()
        authStateHolder.setUser(null)
        authStateHolder.setState(AuthState.LOGGED_OUT)
    }

    /** Drop all local cache + the cached-user marker so nothing leaks past sign-out. */
    private suspend fun wipeCache() {
        cacheCleaner.clear()
        settingsStore.setCachedUserId(null)
    }

    private fun applyUser(user: User) {
        authStateHolder.setUser(user)
        authStateHolder.setState(
            if (user.mustChangePassword) AuthState.NEEDS_PASSWORD_CHANGE else AuthState.AUTHENTICATED,
        )
    }
}
