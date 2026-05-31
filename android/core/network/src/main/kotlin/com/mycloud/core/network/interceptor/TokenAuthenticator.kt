package com.mycloud.core.network.interceptor

import com.mycloud.core.network.auth.TokenProvider
import okhttp3.Authenticator
import okhttp3.Request
import okhttp3.Response
import okhttp3.Route

/**
 * Single-flight refresh on 401. The first 401 acquires the monitor and refreshes
 * (rotating the pair, same family); concurrent 401s that arrive after another
 * thread already rotated simply retry with the new token. A 401 that recurs
 * after one retry, or a failed refresh (e.g. family revoked), forces logout.
 *
 * Wired into the OkHttp client in Phase 1 once a [TokenProvider] is bound.
 */
class TokenAuthenticator(
    private val tokenProvider: TokenProvider,
) : Authenticator {

    override fun authenticate(route: Route?, response: Response): Request? {
        if (responseCount(response) >= MAX_ATTEMPTS) return null

        synchronized(this) {
            val current = tokenProvider.currentAccessToken()
            val failedToken = response.request.header("Authorization")
                ?.removePrefix("Bearer ")
                ?.trim()

            // Another thread already refreshed → just retry with the new token.
            if (current != null && current != failedToken) {
                return response.request.retryWith(current)
            }

            return if (tokenProvider.refreshSync()) {
                val refreshed = tokenProvider.currentAccessToken() ?: return null
                response.request.retryWith(refreshed)
            } else {
                tokenProvider.onRefreshFailed()
                null
            }
        }
    }

    private fun Request.retryWith(token: String): Request =
        newBuilder().header("Authorization", "Bearer $token").build()

    private fun responseCount(response: Response): Int {
        var count = 1
        var prior = response.priorResponse
        while (prior != null) {
            count++
            prior = prior.priorResponse
        }
        return count
    }

    private companion object {
        const val MAX_ATTEMPTS = 2
    }
}
