package com.mycloud.core.common.result

import kotlin.time.Duration

/**
 * The outcome of a single backend call. Repositories return this; ViewModels
 * map it to their screen [androidx.compose.runtime.State]. Mirrors the backend's
 * `{data, meta}` / `{error:{code,message,details}}` envelope plus the transport
 * and auth conditions the client must react to specially.
 */
sealed interface NetworkResult<out T> {

    /** 2xx with a decoded body. [meta] carries pagination cursors when present. */
    data class Success<out T>(val data: T, val meta: Meta? = null) : NetworkResult<T>

    /** A structured `{error:{...}}` envelope from the server (4xx/5xx). */
    data class ApiError(
        val code: String,
        val message: String,
        val httpStatus: Int,
        /** Raw JSON of `error.details`, if any — parsed lazily by callers that care. */
        val details: String? = null,
    ) : NetworkResult<Nothing>

    /** Transport failure (no/garbled response): offline, DNS, TLS, timeout. */
    data class NetworkError(val cause: Throwable) : NetworkResult<Nothing>

    /** 401 that survived the refresh path → the session is gone; force logout. */
    data object Unauthorized : NetworkResult<Nothing>

    /** 429 / 503. [retryAfter] is parsed from the `Retry-After` header if sent. */
    data class RateLimited(val retryAfter: Duration? = null) : NetworkResult<Nothing>

    /** 403 `password_change_required` — gate the app into Change Password. */
    data object PasswordChangeRequired : NetworkResult<Nothing>
}

/** Pagination metadata lifted from the response `meta` block. */
data class Meta(
    val nextCursor: String? = null,
    val total: Int? = null,
)

/** Transform the success payload, leaving every non-success branch untouched. */
inline fun <T, R> NetworkResult<T>.map(transform: (T) -> R): NetworkResult<R> =
    when (this) {
        is NetworkResult.Success -> NetworkResult.Success(transform(data), meta)
        is NetworkResult.ApiError -> this
        is NetworkResult.NetworkError -> this
        NetworkResult.Unauthorized -> NetworkResult.Unauthorized
        is NetworkResult.RateLimited -> this
        NetworkResult.PasswordChangeRequired -> NetworkResult.PasswordChangeRequired
    }

/** The decoded body on success, or null on any failure branch. */
fun <T> NetworkResult<T>.getOrNull(): T? = (this as? NetworkResult.Success)?.data
