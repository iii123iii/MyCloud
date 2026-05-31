package com.mycloud.core.designsystem.gallery

import android.content.res.Configuration.UI_MODE_NIGHT_YES
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Star
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.ElevatedCard
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.mycloud.core.designsystem.theme.MyCloudTheme

/**
 * A visual smoke-test of the theme. Open in the Android Studio preview pane to
 * sanity-check the brand color scheme in light and dark before building screens.
 * Grows into the full component gallery as `:core:designsystem` does.
 */
@Composable
private fun ComponentGallery() {
    Surface {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("MyCloud — Material 3 Expressive")
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = {}) { Text("Primary") }
                FilledTonalButton(onClick = {}) { Text("Tonal") }
                OutlinedButton(onClick = {}) { Text("Outlined") }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                AssistChip(
                    onClick = {},
                    label = { Text("Starred") },
                    leadingIcon = { Icon(Icons.Filled.Star, contentDescription = null) },
                )
                FloatingActionButton(onClick = {}) {
                    Icon(Icons.Filled.Add, contentDescription = "Upload")
                }
            }
            ElevatedCard {
                Text(
                    "Elevated card — real Material tonal elevation.",
                    modifier = Modifier.padding(16.dp),
                )
            }
            LinearProgressIndicator(progress = { 0.6f })
        }
    }
}

@Preview(name = "Light")
@Preview(name = "Dark", uiMode = UI_MODE_NIGHT_YES)
@Composable
private fun ComponentGalleryPreview() {
    MyCloudTheme {
        ComponentGallery()
    }
}
