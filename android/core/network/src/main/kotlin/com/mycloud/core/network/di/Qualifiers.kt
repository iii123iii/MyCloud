package com.mycloud.core.network.di

import javax.inject.Qualifier

/** The backend base URL (with trailing slash). Bound in :app from BuildConfig. */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class BaseUrl

/** The bare OkHttp client/Retrofit with no auth interceptor or authenticator —
 *  used only for the token-refresh call to avoid authenticator recursion. */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class Unauthenticated

/** True for debug builds — gates the insecure (self-signed) TLS shortcut. */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class DebugBuild
