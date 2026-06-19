package com.mycloud.core.designsystem.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ExperimentalMaterial3ExpressiveApi
import androidx.compose.material3.MaterialExpressiveTheme
import androidx.compose.material3.MotionScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext

/**
 * The app theme: Material 3 Expressive with the MyCloud brand color scheme,
 * shapes, type, and the expressive motion scheme (springier, more characterful
 * transitions across every component that reads MotionScheme).
 *
 * Wrap every screen (and Compose preview) in this. Dynamic color is intentionally
 * not used so the brand identity is consistent across devices.
 *
 * The expressive APIs are public from material3 1.5.0-alpha; we pin 1.5.0-alpha14
 * (the highest still on Compose 1.8.x, so no AGP/compileSdk bump is needed).
 */
@OptIn(ExperimentalMaterial3ExpressiveApi::class)
@Composable
fun MyCloudTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    // Off by default to keep the brand identity consistent across devices. Opt in
    // (Android 12+) to follow the user's wallpaper-derived dynamic palette.
    dynamicColor: Boolean = false,
    content: @Composable () -> Unit,
) {
    val colorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> {
            val context = LocalContext.current
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        }
        darkTheme -> DarkColors
        else -> LightColors
    }
    MaterialExpressiveTheme(
        colorScheme = colorScheme,
        motionScheme = MotionScheme.expressive(),
        typography = MyCloudTypography,
        shapes = MyCloudShapes,
        content = content,
    )
}
