package com.mycloud.android.di

import com.mycloud.android.cache.SessionCacheCleanerImpl
import com.mycloud.data.session.SessionCacheCleaner
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Singleton

/** Binds the app-level cache wiper (which needs the Coil singleton) to the
 *  `:data` interface the auth layer triggers on sign-out / user-switch. */
@Module
@InstallIn(SingletonComponent::class)
abstract class CacheModule {

    @Binds
    @Singleton
    abstract fun bindSessionCacheCleaner(impl: SessionCacheCleanerImpl): SessionCacheCleaner
}
