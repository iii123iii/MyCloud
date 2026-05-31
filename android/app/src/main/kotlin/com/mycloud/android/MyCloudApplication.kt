package com.mycloud.android

import android.app.Application
import androidx.hilt.work.HiltWorkerFactory
import androidx.work.Configuration
import coil3.ImageLoader
import coil3.PlatformContext
import coil3.SingletonImageLoader
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import dagger.hilt.android.HiltAndroidApp
import okhttp3.OkHttpClient
import timber.log.Timber
import javax.inject.Inject

/**
 * Application entry point. Hilt roots its graph here; the singleton Coil
 * [ImageLoader] is built on the app's authenticated [OkHttpClient]; and
 * WorkManager is configured on-demand with the Hilt [HiltWorkerFactory] so the
 * transfer workers can be constructed with their injected dependencies.
 */
@HiltAndroidApp
class MyCloudApplication :
    Application(),
    Configuration.Provider,
    SingletonImageLoader.Factory {

    @Inject
    lateinit var okHttpClient: OkHttpClient

    @Inject
    lateinit var workerFactory: HiltWorkerFactory

    override val workManagerConfiguration: Configuration
        get() = Configuration.Builder()
            .setWorkerFactory(workerFactory)
            .build()

    override fun onCreate() {
        super.onCreate()
        if (BuildConfig.DEBUG) {
            Timber.plant(Timber.DebugTree())
        }
    }

    override fun newImageLoader(context: PlatformContext): ImageLoader =
        ImageLoader.Builder(context)
            .components {
                add(OkHttpNetworkFetcherFactory(callFactory = { okHttpClient }))
            }
            .build()
}
