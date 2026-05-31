package com.mycloud.core.common.result

/** A user-facing message for a failed [NetworkResult], or null on success. */
fun NetworkResult<*>.userMessageOrNull(): String? = when (this) {
    is NetworkResult.Success -> null
    is NetworkResult.ApiError -> message
    is NetworkResult.NetworkError -> "Can't reach the server. Check your connection."
    NetworkResult.Unauthorized -> "Your session has expired. Please sign in again."
    is NetworkResult.RateLimited -> "Too many requests. Please try again shortly."
    NetworkResult.PasswordChangeRequired -> "You must change your password to continue."
}
