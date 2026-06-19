package com.mycloud.android.di

import com.mycloud.android.BuildConfig
import com.mycloud.core.network.di.AppVersionName
import com.mycloud.core.network.di.BaseUrl
import com.mycloud.core.network.di.DebugBuild
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** App-level bindings. Supplies the backend base URL + debug flag to :core:network. */
@Module
@InstallIn(SingletonComponent::class)
object AppModule {

    @Provides
    @Singleton
    @BaseUrl
    fun provideBaseUrl(): String = BuildConfig.BASE_URL

    @Provides
    @DebugBuild
    fun provideIsDebugBuild(): Boolean = BuildConfig.DEBUG

    @Provides
    @AppVersionName
    fun provideAppVersionName(): String = BuildConfig.VERSION_NAME
}
