package com.mycloud.android.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.android.RootViewModel
import com.mycloud.core.model.AuthState
import com.mycloud.feature.auth.lock.AppLockScreen
import com.mycloud.feature.auth.login.LoginScreen
import com.mycloud.feature.auth.password.ChangePasswordScreen

/**
 * Root navigation gate. Renders the right top-level surface for the current
 * [AuthState]; signals [onReady] once state resolves so the splash can dismiss.
 * The Authenticated branch hosts the [AppShell].
 */
@Composable
fun RootContent(
    onReady: () -> Unit,
    viewModel: RootViewModel = hiltViewModel(),
) {
    val gate by viewModel.gate.collectAsStateWithLifecycle()

    LaunchedEffect(gate) {
        if (gate != AuthState.LOADING) onReady()
    }

    when (gate) {
        AuthState.LOADING -> Box(Modifier.fillMaxSize())
        AuthState.LOGGED_OUT -> LoginScreen()
        AuthState.NEEDS_PASSWORD_CHANGE -> ChangePasswordScreen()
        AuthState.LOCKED -> AppLockScreen()
        AuthState.AUTHENTICATED -> AppShell(onSignOut = viewModel::logout)
    }
}
