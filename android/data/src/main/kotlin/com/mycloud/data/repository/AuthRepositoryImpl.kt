package com.mycloud.data.repository

import android.os.Build
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.map
import com.mycloud.core.datastore.settings.ServerConfig
import com.mycloud.core.datastore.settings.SettingsStore
import com.mycloud.core.datastore.token.SecureTokenStore
import com.mycloud.core.model.AuthState
import com.mycloud.core.model.User
import com.mycloud.core.network.api.AuthApi
import com.mycloud.core.network.api.DeviceLinkApi
import com.mycloud.core.network.dto.AuthTokensDto
import com.mycloud.core.network.dto.ChangePasswordRequestDto
import com.mycloud.core.network.dto.DeleteAccountRequestDto
import com.mycloud.core.network.dto.DeviceLinkClaimRequestDto
import com.mycloud.core.network.dto.DeviceLinkPollDto
import com.mycloud.core.network.dto.DeviceLinkPollRequestDto
import com.mycloud.core.network.dto.LoginRequestDto
import com.mycloud.core.network.dto.LogoutRequestDto
import com.mycloud.core.network.result.safeApiCall
import com.mycloud.core.network.result.safeUnitApiCall
import com.mycloud.core.work.TransferManager
import com.mycloud.data.mapper.toDomain
import com.mycloud.data.mapper.toTokenBundle
import com.mycloud.data.mapper.toUser
import com.mycloud.data.session.AuthStateHolder
import com.mycloud.data.session.SessionCacheCleaner
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import java.time.Instant
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class AuthRepositoryImpl @Inject constructor(
    private val authApi: AuthApi,
    private val deviceLinkApi: DeviceLinkApi,
    private val tokenStore: SecureTokenStore,
    private val authStateHolder: AuthStateHolder,
    private val serverConfig: ServerConfig,
    private val settingsStore: SettingsStore,
    private val cacheCleaner: SessionCacheCleaner,
    private val json: Json,
    private val transferManager: TransferManager,
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

    override suspend fun linkViaQr(
        serverUrl: String,
        code: String,
        verifier: String,
    ): NetworkResult<Unit> {
        val previousUrl = serverConfig.current()
        var linked = false
        return try {
            // Point the client at the scanned server first so claim/poll hit it.
            serverConfig.setUrl(serverUrl)

            val deviceModel = Build.MODEL?.takeIf { it.isNotBlank() } ?: "Android device"
            val platform = "Android ${Build.VERSION.RELEASE}"
            val claim = safeApiCall(json) {
                deviceLinkApi.claim(
                    DeviceLinkClaimRequestDto(
                        code = code,
                        verifier = verifier,
                        deviceName = deviceModel,
                        deviceModel = deviceModel,
                        platform = platform,
                    ),
                )
            }
            if (claim !is NetworkResult.Success) return claim.map { }

            // Poll until the browser approves (tokens arrive), denies, or the code
            // expires. The backend keeps the pairing alive ~120s after a scan.
            val deadline = System.currentTimeMillis() + POLL_WINDOW_MS
            while (System.currentTimeMillis() < deadline) {
                when (val poll = safeApiCall(json) {
                    deviceLinkApi.poll(DeviceLinkPollRequestDto(code, verifier))
                }) {
                    is NetworkResult.Success -> {
                    val data = poll.data
                    if (data.hasTokens) {
                        val linkResult = finishLink(data)
                        if (linkResult is NetworkResult.Success) linked = true
                        return linkResult
                    }
                        when (data.state) {
                            "denied" -> return NetworkResult.ApiError(
                                "device_link_denied", "Sign-in was denied on the other device.", 403,
                            )
                            "expired" -> return NetworkResult.ApiError(
                                "device_link_expired", "The QR code expired. Generate a new one.", 410,
                            )
                            // pending / awaiting_approval - keep waiting.
                        }
                    }
                    // Transport/HTTP failure (e.g. bad verifier 403) - surface it.
                    else -> return poll.map { }
                }
                delay(POLL_INTERVAL_MS)
            }
            NetworkResult.ApiError(
                "device_link_timeout", "Timed out waiting for approval. Try again.", 408,
            )
        } finally {
            if (!linked) {
                withContext(NonCancellable) {
                    serverConfig.setUrl(previousUrl)
                }
            }
        }
    }

    /** Persist the QR-delivered tokens and flip the session to authenticated. */
    private suspend fun finishLink(data: DeviceLinkPollDto): NetworkResult<Unit> {
        val validationError = validateLinkTokens(data)
        if (validationError != null) return validationError

        val tokens = AuthTokensDto(
            accessToken = data.accessToken.orEmpty(),
            refreshToken = data.refreshToken.orEmpty(),
            accessJti = data.accessJti,
            refreshJti = data.refreshJti,
            family = data.family.orEmpty(),
            accessExpiresAt = data.accessExpiresAt.orEmpty(),
            userId = data.userId.orEmpty(),
            role = data.role ?: "user",
            username = data.username,
            email = data.email,
            mustChangePassword = data.mustChangePassword,
        )
        // Same cache-isolation rule as password login: a different user must not
        // inherit the previous user's cached files.
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
        return NetworkResult.Success(Unit)
    }

    private fun validateLinkTokens(data: DeviceLinkPollDto): NetworkResult.ApiError? {
        if (data.accessToken.isNullOrBlank() ||
            data.refreshToken.isNullOrBlank() ||
            data.family.isNullOrBlank() ||
            data.accessExpiresAt.isNullOrBlank() ||
            data.userId.isNullOrBlank()
        ) {
            return NetworkResult.ApiError(
                "device_link_invalid_tokens",
                "The server returned an incomplete sign-in response.",
                502,
            )
        }
        val expiresAt = runCatching { Instant.parse(data.accessExpiresAt) }.getOrNull()
        if (expiresAt == null) {
            return NetworkResult.ApiError(
                "device_link_invalid_tokens",
                "The server returned an invalid sign-in response.",
                502,
            )
        }
        return null
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
        // Cancel/prune transfers so the next signed-in user doesn't see old jobs
        // or receive completions from the signed-out account.
        runCatching { transferManager.cancelAndPruneTransfers() }
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

    private companion object {
        // Matches the backend's post-scan approval TTL (~120s) plus a little slack.
        const val POLL_WINDOW_MS = 125_000L
        const val POLL_INTERVAL_MS = 2_000L
    }
}
