package com.mycloud.core.network.interceptor

import com.mycloud.core.network.auth.TokenProvider
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Attaches `Authorization: Bearer <access>` to authenticated requests. The
 * unauthenticated allowlist (login/refresh/register/public/dl/setup) is skipped
 * so those calls never carry a stale token.
 *
 * Wired into the OkHttp client in Phase 1 once a [TokenProvider] is bound.
 */
class AuthInterceptor(
    private val tokenProvider: TokenProvider,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (isAuthFree(request.url.encodedPath)) {
            return chain.proceed(request)
        }
        // Proactively refresh just before expiry so the session never visibly lapses;
        // the TokenAuthenticator remains the fallback for an unexpected 401.
        // expiresAt <= 0 means "unknown" (parse failure) — don't treat it as
        // "already expired", or we'd refresh before every single request.
        val expiresAt = tokenProvider.accessExpiresAtMillis()
        if (expiresAt != null && expiresAt > 0 && System.currentTimeMillis() >= expiresAt - REFRESH_SKEW_MS) {
            tokenProvider.refreshSync()
        }
        val token = tokenProvider.currentAccessToken()
            ?: return chain.proceed(request)
        val authed = request.newBuilder()
            .header("Authorization", "Bearer $token")
            .build()
        return chain.proceed(authed)
    }

    private fun isAuthFree(path: String): Boolean =
        AUTH_FREE_PREFIXES.any { path.contains(it) }

    private companion object {
        const val REFRESH_SKEW_MS = 60_000L
        val AUTH_FREE_PREFIXES = listOf(
            "/auth/login",
            "/auth/refresh",
            "/auth/register",
            "/setup/",
            "/public/",
            "/dl/",
        )
    }
}
