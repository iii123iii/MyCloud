package com.mycloud.core.network.di

import com.mycloud.core.network.api.AccountApi
import com.mycloud.core.network.api.AuthApi
import com.mycloud.core.network.api.CommentApi
import com.mycloud.core.network.api.FileApi
import com.mycloud.core.network.api.FolderApi
import com.mycloud.core.network.api.PhotoApi
import com.mycloud.core.network.api.ShareApi
import com.mycloud.core.network.api.StorageApi
import com.mycloud.core.network.api.TokenRefreshApi
import com.mycloud.core.network.api.TransferApi
import com.mycloud.core.network.api.TrashApi
import com.mycloud.core.network.api.VersionApi
import com.mycloud.core.network.auth.TokenProvider
import com.mycloud.core.network.config.ServerUrlSource
import com.mycloud.core.network.interceptor.AuthInterceptor
import com.mycloud.core.network.interceptor.HostSelectionInterceptor
import com.mycloud.core.network.interceptor.RetryInterceptor
import com.mycloud.core.network.interceptor.TokenAuthenticator
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNamingStrategy
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.security.SecureRandom
import java.security.cert.X509Certificate
import java.util.concurrent.TimeUnit
import javax.inject.Singleton
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {

    @Provides
    @Singleton
    @OptIn(ExperimentalSerializationApi::class)
    fun provideJson(): Json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
        // DTOs mirror the backend's snake_case without per-field @SerialName.
        namingStrategy = JsonNamingStrategy.SnakeCase
    }

    private fun loggingInterceptor(): HttpLoggingInterceptor =
        HttpLoggingInterceptor().apply { level = HttpLoggingInterceptor.Level.BASIC }

    // --- Bare client used only for token refresh (no authenticator → no recursion). ---

    @Provides
    @Singleton
    @Unauthenticated
    fun provideBareClient(
        serverUrlSource: ServerUrlSource,
        @DebugBuild debug: Boolean,
    ): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .addInterceptor(HostSelectionInterceptor(serverUrlSource))
        .addInterceptor(loggingInterceptor())
        .apply { if (debug) allowInsecureTls() }
        .build()

    @Provides
    @Singleton
    @Unauthenticated
    fun provideBareRetrofit(
        @BaseUrl baseUrl: String,
        @Unauthenticated client: OkHttpClient,
        json: Json,
    ): Retrofit = retrofitOf(baseUrl, client, json)

    @Provides
    @Singleton
    fun provideTokenRefreshApi(@Unauthenticated retrofit: Retrofit): TokenRefreshApi =
        retrofit.create(TokenRefreshApi::class.java)

    // --- Main authenticated client: bearer attach + retry + single-flight refresh. ---

    @Provides
    @Singleton
    fun provideOkHttpClient(
        tokenProvider: TokenProvider,
        serverUrlSource: ServerUrlSource,
        @DebugBuild debug: Boolean,
    ): OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .writeTimeout(60, TimeUnit.SECONDS)
        .apply { if (debug) allowInsecureTls() }
        .addInterceptor(HostSelectionInterceptor(serverUrlSource))
        .addInterceptor(AuthInterceptor(tokenProvider))
        .addInterceptor(RetryInterceptor())
        .addInterceptor(loggingInterceptor())
        .authenticator(TokenAuthenticator(tokenProvider))
        .build()

    @Provides
    @Singleton
    fun provideRetrofit(
        @BaseUrl baseUrl: String,
        client: OkHttpClient,
        json: Json,
    ): Retrofit = retrofitOf(baseUrl, client, json)

    @Provides
    @Singleton
    fun provideAuthApi(retrofit: Retrofit): AuthApi = retrofit.create(AuthApi::class.java)

    @Provides
    @Singleton
    fun provideFileApi(retrofit: Retrofit): FileApi = retrofit.create(FileApi::class.java)

    @Provides
    @Singleton
    fun provideFolderApi(retrofit: Retrofit): FolderApi = retrofit.create(FolderApi::class.java)

    @Provides
    @Singleton
    fun provideStorageApi(retrofit: Retrofit): StorageApi = retrofit.create(StorageApi::class.java)

    @Provides
    @Singleton
    fun provideTransferApi(retrofit: Retrofit): TransferApi = retrofit.create(TransferApi::class.java)

    @Provides
    @Singleton
    fun provideShareApi(retrofit: Retrofit): ShareApi = retrofit.create(ShareApi::class.java)

    @Provides
    @Singleton
    fun provideTrashApi(retrofit: Retrofit): TrashApi = retrofit.create(TrashApi::class.java)

    @Provides
    @Singleton
    fun providePhotoApi(retrofit: Retrofit): PhotoApi = retrofit.create(PhotoApi::class.java)

    @Provides
    @Singleton
    fun provideCommentApi(retrofit: Retrofit): CommentApi = retrofit.create(CommentApi::class.java)

    @Provides
    @Singleton
    fun provideVersionApi(retrofit: Retrofit): VersionApi = retrofit.create(VersionApi::class.java)

    @Provides
    @Singleton
    fun provideAccountApi(retrofit: Retrofit): AccountApi = retrofit.create(AccountApi::class.java)

    private fun retrofitOf(baseUrl: String, client: OkHttpClient, json: Json): Retrofit {
        val contentType = "application/json".toMediaType()
        return Retrofit.Builder()
            .baseUrl(baseUrl)
            .client(client)
            .addConverterFactory(json.asConverterFactory(contentType))
            .build()
    }
}

/**
 * DEBUG ONLY: trust any TLS cert and skip hostname verification, so the app can
 * reach a local self-signed server (e.g. nginx at https://10.0.2.2). Never enabled
 * in release builds — gated by the [DebugBuild] flag at the call site.
 */
private fun OkHttpClient.Builder.allowInsecureTls(): OkHttpClient.Builder {
    val trustManager = object : X509TrustManager {
        override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {}
        override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
    }
    val sslContext = SSLContext.getInstance("TLS").apply {
        init(null, arrayOf<TrustManager>(trustManager), SecureRandom())
    }
    sslSocketFactory(sslContext.socketFactory, trustManager)
    hostnameVerifier { _, _ -> true }
    return this
}
