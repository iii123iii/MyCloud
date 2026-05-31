package com.mycloud.data.auth

import com.mycloud.core.datastore.token.SecureTokenStore
import com.mycloud.core.model.AuthState
import com.mycloud.core.network.api.TokenRefreshApi
import com.mycloud.core.network.auth.TokenProvider
import com.mycloud.core.network.dto.RefreshRequestDto
import com.mycloud.data.mapper.toTokenBundle
import com.mycloud.data.session.AuthStateHolder
import com.mycloud.data.session.SessionCacheCleaner
import kotlinx.coroutines.runBlocking
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Bridges the OkHttp auth layer to the encrypted store + refresh endpoint.
 * [refreshSync] is blocking because OkHttp's [okhttp3.Authenticator] runs on a
 * background dispatcher; the [com.mycloud.core.network.interceptor.TokenAuthenticator]
 * already serialises callers, so concurrent 401s won't refresh more than once.
 */
@Singleton
class TokenProviderImpl @Inject constructor(
    private val tokenStore: SecureTokenStore,
    private val refreshApi: TokenRefreshApi,
    private val authStateHolder: AuthStateHolder,
    private val cacheCleaner: SessionCacheCleaner,
) : TokenProvider {

    private val refreshLock = Any()

    override fun currentAccessToken(): String? = tokenStore.currentAccessToken()

    override fun accessExpiresAtMillis(): Long? = tokenStore.currentBundle()?.accessExpiresAtMillis

    override fun refreshSync(): Boolean {
        // Snapshot before locking; if another caller rotated while we waited, reuse it.
        val staleToken = tokenStore.currentAccessToken()
        synchronized(refreshLock) {
            val now = tokenStore.currentAccessToken()
            if (now != null && now != staleToken) return true
            val refreshToken = tokenStore.currentRefreshToken() ?: return false
            return runBlocking {
                val response = runCatching { refreshApi.refresh(RefreshRequestDto(refreshToken)) }
                    .getOrNull() ?: return@runBlocking false
                val tokens = if (response.isSuccessful) response.body()?.data else null
                if (tokens != null) {
                    tokenStore.save(tokens.toTokenBundle())
                    true
                } else {
                    false
                }
            }
        }
    }

    override fun onRefreshFailed() {
        // Involuntary sign-out (refresh token rejected/expired): clear tokens AND
        // wipe the local cache so the next user on the device can't read it.
        runBlocking {
            tokenStore.clear()
            cacheCleaner.clear()
        }
        authStateHolder.setUser(null)
        authStateHolder.setState(AuthState.LOGGED_OUT)
    }
}
