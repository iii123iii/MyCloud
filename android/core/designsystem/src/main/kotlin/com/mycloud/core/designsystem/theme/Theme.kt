package com.mycloud.core.designsystem.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable

/**
 * The app theme: Material 3 with the MyCloud brand color scheme, shapes, and type.
 *
 * Wrap every screen (and Compose preview) in this. Dynamic color is intentionally
 * not used so the brand identity is consistent across devices.
 *
 * NOTE: the Expressive theme wrapper (`MaterialExpressiveTheme` + `MotionScheme`)
 * is `internal` in material3 1.4.0; bump material3 to a version that exposes it to
 * switch back to `MaterialExpressiveTheme(...)`. The components used throughout are
 * standard Material 3, so the look is unaffected by using `MaterialTheme` here.
 */
@Composable
fun MyCloudTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = if (darkTheme) DarkColors else LightColors,
        typography = MyCloudTypography,
        shapes = MyCloudShapes,
        content = content,
    )
}
