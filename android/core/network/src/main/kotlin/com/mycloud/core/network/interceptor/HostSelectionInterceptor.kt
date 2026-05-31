package com.mycloud.core.network.interceptor

import com.mycloud.core.network.config.ServerUrlSource
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Rewrites each request's scheme/host/port to the user-configured server, so the
 * Retrofit base URL is only a placeholder and the server address can change at
 * runtime. The request path (e.g. `api/v2/...`) is preserved.
 */
class HostSelectionInterceptor(
    private val serverUrlSource: ServerUrlSource,
) : Interceptor {

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        val base = serverUrlSource.currentBaseUrl().toHttpUrlOrNull()
            ?: return chain.proceed(request)
        val newUrl = request.url.newBuilder()
            .scheme(base.scheme)
            .host(base.host)
            .port(base.port)
            .build()
        return chain.proceed(request.newBuilder().url(newUrl).build())
    }
}
