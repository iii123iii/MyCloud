package com.mycloud.android.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.datastore.settings.SettingsStore
import com.mycloud.core.datastore.settings.ThemeMode
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import javax.inject.Inject

/**
 * Activity-scoped holder for the chosen [ThemeMode] so [MainActivity] can pick the
 * light/dark color scheme above the navigation graph. Eagerly started so the very
 * first frame already reflects the persisted preference (no light-mode flash).
 */
@HiltViewModel
class ThemeViewModel @Inject constructor(
    settingsStore: SettingsStore,
) : ViewModel() {

    val themeMode: StateFlow<ThemeMode> = settingsStore.themeMode
        .stateIn(viewModelScope, SharingStarted.Eagerly, ThemeMode.SYSTEM)
}
