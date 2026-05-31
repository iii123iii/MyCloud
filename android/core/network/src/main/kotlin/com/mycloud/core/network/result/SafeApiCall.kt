package com.mycloud.core.network.result

import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.network.envelope.Envelope
import com.mycloud.core.network.envelope.ErrorBody
import kotlinx.serialization.json.Json
import okhttp3.Headers
import retrofit2.Response
import java.io.IOException
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

/**
 * Runs a Retrofit call and folds the HTTP/transport outcome into a
 * [NetworkResult]. By the time a 401 reaches here the OkHttp [TokenAuthenticator]
 * has already tried (and failed) to refresh, so 401 maps to [NetworkResult.Unauthorized].
 *
 * Service methods should return `Response<Envelope<T>>` so status codes and the
 * `Retry-After` header are visible.
 */
suspend fun <T> safeApiCall(
    json: Json,
    call: suspend () -> Response<Envelope<T>>,
): NetworkResult<T> = try {
    val response = call()
    if (response.isSuccessful) {
        val data = response.body()?.data
        if (data != null) {
            NetworkResult.Success(data, response.body()?.meta?.toMeta())
        } else {
            NetworkResult.ApiError("empty_body", "Empty response body", response.code())
        }
    } else {
        mapError(response.code(), parseError(json, response), response.headers())
    }
} catch (io: IOException) {
    NetworkResult.NetworkError(io)
} catch (t: Throwable) {
    NetworkResult.NetworkError(t)
}

/** Variant for 204 / no-content endpoints (delete, restore, …). */
suspend fun safeUnitApiCall(
    json: Json,
    call: suspend () -> Response<Unit>,
): NetworkResult<Unit> = try {
    val response = call()
    if (response.isSuccessful) {
        NetworkResult.Success(Unit)
    } else {
        mapError(response.code(), parseErrorBytes(json, response.errorBody()?.string()), response.headers())
    }
} catch (io: IOException) {
    NetworkResult.NetworkError(io)
} catch (t: Throwable) {
    NetworkResult.NetworkError(t)
}

private fun parseError(json: Json, response: Response<*>): ErrorBody? =
    parseErrorBytes(json, response.errorBody()?.string())

private fun parseErrorBytes(json: Json, raw: String?): ErrorBody? {
    if (raw.isNullOrBlank()) return null
    return runCatching {
        json.decodeFromString(Envelope.serializer(kotlinx.serialization.json.JsonElement.serializer()), raw).error
    }.getOrNull()
}

private fun mapError(code: Int, error: ErrorBody?, headers: Headers): NetworkResult<Nothing> {
    val errorCode = error?.code.orEmpty()
    return when {
        code == 401 -> NetworkResult.Unauthorized
        code == 403 && errorCode == "password_change_required" -> NetworkResult.PasswordChangeRequired
        code == 429 || code == 503 -> NetworkResult.RateLimited(parseRetryAfter(headers))
        else -> NetworkResult.ApiError(
            code = errorCode.ifEmpty { "http_$code" },
            message = error?.message?.ifEmpty { null } ?: "Request failed ($code)",
            httpStatus = code,
            details = error?.details?.toString(),
        )
    }
}

private fun parseRetryAfter(headers: Headers): Duration? =
    headers["Retry-After"]?.toLongOrNull()?.seconds
