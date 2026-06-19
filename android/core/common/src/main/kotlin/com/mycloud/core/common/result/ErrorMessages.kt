package com.mycloud.core.common.result

/** A user-facing message for a failed [NetworkResult], or null on success. */
fun NetworkResult<*>.userMessageOrNull(): String? = when (this) {
    is NetworkResult.Success -> null
    is NetworkResult.ApiError -> sanitizeServerMessage(message)
    is NetworkResult.NetworkError -> networkErrorMessage(cause)
    NetworkResult.Unauthorized -> "Your session has expired. Please sign in again."
    is NetworkResult.RateLimited -> "Too many requests. Please try again shortly."
    NetworkResult.PasswordChangeRequired -> "You must change your password to continue."
}

/** Generic fallback shown instead of a missing/garbage/oversized server message. */
private const val GENERIC_ERROR = "Something went wrong. Please try again."
private const val MAX_MESSAGE_LENGTH = 200

/**
 * Keeps a concise, human-looking server message; otherwise falls back to a
 * generic line. Guards against blank messages, raw stack traces / HTML pages
 * leaking through, and unbounded length blowing up the UI.
 */
private fun sanitizeServerMessage(raw: String?): String {
    val msg = raw?.trim().orEmpty()
    if (msg.isEmpty()) return GENERIC_ERROR
    // Reject obvious non-message payloads (HTML error pages, JSON blobs, traces).
    if (msg.startsWith("<") || msg.startsWith("{") || msg.startsWith("[") ||
        msg.contains("Exception") || msg.contains("\n")
    ) {
        return GENERIC_ERROR
    }
    return if (msg.length > MAX_MESSAGE_LENGTH) {
        msg.take(MAX_MESSAGE_LENGTH).trimEnd() + "…"
    } else {
        msg
    }
}

/**
 * Turns a transport exception into a message that names the actual problem, so a
 * misconfigured server address is diagnosable instead of a blanket "check your
 * connection" (which hid DNS / wrong-port / wrong-scheme failures).
 */
private fun networkErrorMessage(cause: Throwable): String {
    val name = cause::class.simpleName.orEmpty()
    val detail = cause.message?.takeIf { it.isNotBlank() }
    return when {
        name.contains("UnknownHost", ignoreCase = true) ->
            "Can't find that server address. Double-check the host/domain." +
                (detail?.let { "\n($it)" } ?: "")
        name.contains("ConnectException", ignoreCase = true) ||
            name.contains("Timeout", ignoreCase = true) ->
            "Couldn't connect to the server. Check it's running, reachable on this " +
                "network, and that the port/scheme (http vs https) is right." +
                (detail?.let { "\n($it)" } ?: "")
        name.contains("SSL", ignoreCase = true) ->
            "Secure (TLS) connection failed." + (detail?.let { "\n($it)" } ?: "")
        detail != null -> "Can't reach the server.\n$detail"
        else -> "Can't reach the server. Check the address and your connection."
    }
}
