package com.mycloud.core.network.interceptor

import android.os.Build
import okhttp3.Interceptor
import okhttp3.Response

/**
 * Tags every outbound request with a recognisable User-Agent so the backend's
 * session list shows e.g. "MyCloud app on Android — Pixel 8" instead of the
 * "Unknown browser on Unknown OS" fallback it derives when no UA is sent.
 *
 * Format: `MyCloud-Android/<appVersion> (Android <release>; <model>)`
 * The leading `mycloud-android` product token is what the backend's
 * deviceLabelFromUA matches on.
 *
 * Installed on BOTH OkHttp clients (authenticated + bare) so token refresh and
 * the unauthenticated QR claim/poll calls are labelled too. Only set when the
 * caller hasn't already provided a User-Agent.
 */
class UserAgentInterceptor(
    appVersionName: String,
) : Interceptor {

    private val userAgent: String =
        "MyCloud-Android/$appVersionName (Android ${Build.VERSION.RELEASE}; ${Build.MODEL})"

    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (request.header("User-Agent") != null) {
            return chain.proceed(request)
        }
        return chain.proceed(
            request.newBuilder()
                .header("User-Agent", userAgent)
                .build(),
        )
    }
}
