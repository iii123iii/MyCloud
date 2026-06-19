package com.mycloud.feature.photos

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.model.Photo
import com.mycloud.data.repository.PhotoRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant
import java.time.YearMonth
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import javax.inject.Inject

data class PhotoSection(val title: String, val photos: List<Photo>)

data class PhotosUiState(
    val sections: List<PhotoSection> = emptyList(),
    val isLoading: Boolean = false,
    val error: String? = null,
)

@HiltViewModel
class PhotosViewModel @Inject constructor(
    private val photoRepository: PhotoRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(PhotosUiState())
    val state: StateFlow<PhotosUiState> = _state.asStateFlow()

    // The screen's LaunchedEffect(Unit) triggers the initial load; no init{} call
    // so the VM doesn't fetch twice on first composition.

    fun refresh() {
        _state.update { it.copy(isLoading = true) }
        viewModelScope.launch {
            when (val result = photoRepository.photos()) {
                is NetworkResult.Success ->
                    _state.update {
                        it.copy(sections = groupByMonth(result.data), isLoading = false, error = null)
                    }
                else ->
                    // Keep the previously loaded sections so a transient failure
                    // doesn't blank the timeline; just surface the error.
                    _state.update { it.copy(isLoading = false, error = result.userMessageOrNull()) }
            }
        }
    }

    private fun groupByMonth(photos: List<Photo>): List<PhotoSection> {
        // Built per refresh so a locale/timezone change since construction is honoured.
        val monthFormat = DateTimeFormatter.ofPattern("MMMM yyyy", Locale.getDefault())
        val zone = ZoneId.systemDefault()
        val (dated, undated) = photos.partition { it.takenAtMillis > 0 }
        val datedSections = dated.sortedByDescending { it.takenAtMillis }
            .groupBy { yearMonthOf(it.takenAtMillis, zone) }
            .toSortedMap(compareByDescending { it })
            .map { (ym, list) -> PhotoSection(title = monthFormat.format(ym), photos = list) }
        return if (undated.isEmpty()) {
            datedSections
        } else {
            datedSections + PhotoSection(title = "Unknown date", photos = undated)
        }
    }

    private fun yearMonthOf(millis: Long, zone: ZoneId): YearMonth =
        YearMonth.from(Instant.ofEpochMilli(millis).atZone(zone).toLocalDate())

    fun thumbnailUrl(id: String): String = photoRepository.thumbnailUrl(id)
    fun previewUrl(id: String): String = photoRepository.previewUrl(id)
}
