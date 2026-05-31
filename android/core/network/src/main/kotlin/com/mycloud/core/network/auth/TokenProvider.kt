package com.mycloud.core.network.auth

/**
 * The bridge the OkHttp auth layer uses to read and refresh the access token.
 * Implemented in the data/datastore layer (backed by the encrypted token store)
 * and bound into Hilt there. The refresh call is synchronous because OkHttp's
 * [okhttp3.Authenticator] runs on a blocking dispatcher.
 */
interface TokenProvider {

    /** Current access token, or null when signed out. */
    fun currentAccessToken(): String?

    /** Epoch millis when the access token expires, or null when signed out. */
    fun accessExpiresAtMillis(): Long?

    /**
     * Perform a single-flight refresh (POST /auth/refresh) and persist the
     * rotated pair. Returns true if a fresh access token is now available.
     * Safe to call concurrently — dedups if another caller already refreshed.
     */
    fun refreshSync(): Boolean

    /** Called when refresh fails / the token family was revoked → force logout. */
    fun onRefreshFailed()
}
