package com.mycloud.android

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.runtime.SideEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.core.content.ContextCompat
import androidx.core.splashscreen.SplashScreen.Companion.installSplashScreen
import androidx.core.view.WindowCompat
import androidx.fragment.app.FragmentActivity
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.android.ui.RootContent
import com.mycloud.android.ui.ThemeViewModel
import com.mycloud.core.datastore.settings.ThemeMode
import com.mycloud.core.designsystem.theme.MyCloudTheme
import dagger.hilt.android.AndroidEntryPoint

/**
 * Single Activity. Extends [FragmentActivity] so the biometric app-lock prompt
 * can attach. The splash is held until the root gate resolves the auth state.
 */
@AndroidEntryPoint
class MainActivity : FragmentActivity() {

    private val requestNotifications =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { /* best effort */ }

    // Deep link to route once the authenticated shell is shown. Compose state so a
    // new intent (onNewIntent) recomposes and re-routes a running instance.
    private var pendingDeepLink by mutableStateOf<Uri?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        val splash = installSplashScreen()
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()

        pendingDeepLink = intent?.takeIf { it.action == Intent.ACTION_VIEW }?.data

        var authReady by mutableStateOf(false)
        var themeReady by mutableStateOf(false)
        // Hold the splash until both the auth state AND the persisted theme have
        // resolved, so the first painted frame never flashes the seed (SYSTEM) theme.
        splash.setKeepOnScreenCondition { !authReady || !themeReady }

        setContent {
            val themeViewModel: ThemeViewModel = hiltViewModel()
            val themeState by themeViewModel.state.collectAsStateWithLifecycle()
            if (themeState.loaded) themeReady = true
            val darkTheme = when (themeState.mode) {
                ThemeMode.LIGHT -> false
                ThemeMode.DARK -> true
                ThemeMode.SYSTEM -> isSystemInDarkTheme()
            }
            // Keep the (transparent, edge-to-edge) system bars' icons legible: dark
            // icons on light theme, light icons on dark theme — and update live when
            // the in-app theme toggle flips, not just when the OS setting changes.
            // SideEffect (not LaunchedEffect) so the bars are correct on the very
            // first committed frame, avoiding a one-frame flash of wrong-tint icons.
            SideEffect {
                val controller = WindowCompat.getInsetsController(window, window.decorView)
                controller.isAppearanceLightStatusBars = !darkTheme
                controller.isAppearanceLightNavigationBars = !darkTheme
            }
            MyCloudTheme(darkTheme = darkTheme) {
                RootContent(
                    onReady = { authReady = true },
                    onEnterMainShell = ::maybeRequestNotificationPermission,
                    deepLink = pendingDeepLink,
                    onDeepLinkHandled = { pendingDeepLink = null },
                )
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        if (intent.action == Intent.ACTION_VIEW) pendingDeepLink = intent.data
    }

    private fun maybeRequestNotificationPermission() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val granted = ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
        if (!granted) {
            requestNotifications.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }
}
