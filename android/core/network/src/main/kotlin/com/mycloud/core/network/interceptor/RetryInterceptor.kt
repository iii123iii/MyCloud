package com.mycloud.core.network.interceptor

import okhttp3.Interceptor
import okhttp3.Response
import java.io.IOException
import kotlin.math.min

/**
 * Bounded retry for transient failures: IOExceptions and 503/429 responses,
 * with exponential backoff that honors a `Retry-After` header when present.
 * GET/HEAD only by default — retrying non-idempotent writes is unsafe.
 *
 * Wired into the OkHttp client in Phase 1.
 */
class RetryInterceptor(
    private val maxRetries: Int = 2,
    private val baseDelayMillis: Long = 500L,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val idempotent = request.method == "GET" || request.method == "HEAD"

        var attempt = 0
        var lastError: IOException? = null
        while (attempt <= maxRetries) {
            try {
                val response = chain.proceed(request)
                if (idempotent && response.code in RETRYABLE_CODES && attempt < maxRetries) {
                    val wait = retryAfterMillis(response) ?: backoff(attempt)
                    response.close()
                    sleep(wait)
                    attempt++
                    continue
                }
                return response
            } catch (io: IOException) {
                if (!idempotent || attempt >= maxRetries) throw io
                lastError = io
                sleep(backoff(attempt))
                attempt++
            }
        }
        throw lastError ?: IOException("Retry interceptor exhausted")
    }

    private fun backoff(attempt: Int): Long =
        min(baseDelayMillis shl attempt, MAX_DELAY_MILLIS)

    private fun retryAfterMillis(response: Response): Long? =
        response.header("Retry-After")?.toLongOrNull()?.times(1000)

    private fun sleep(millis: Long) {
        try {
            Thread.sleep(millis)
        } catch (e: InterruptedException) {
            Thread.currentThread().interrupt()
        }
    }

    private companion object {
        val RETRYABLE_CODES = setOf(429, 503)
        const val MAX_DELAY_MILLIS = 8_000L
    }
}
