package com.mycloud.feature.auth.lock

import androidx.lifecycle.ViewModel
import com.mycloud.core.datastore.settings.AppLockManager
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

@HiltViewModel
class AppLockViewModel @Inject constructor(
    private val appLockManager: AppLockManager,
) : ViewModel() {

    fun unlock() = appLockManager.unlock()
}
