package com.mycloud.feature.photos

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
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
)

@HiltViewModel
class PhotosViewModel @Inject constructor(
    private val photoRepository: PhotoRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(PhotosUiState())
    val state: StateFlow<PhotosUiState> = _state.asStateFlow()

    private val monthFormat = DateTimeFormatter.ofPattern("MMMM yyyy", Locale.getDefault())
    private val zone = ZoneId.systemDefault()

    init {
        refresh()
    }

    fun refresh() {
        _state.update { it.copy(isLoading = true) }
        viewModelScope.launch {
            val photos = (photoRepository.photos() as? NetworkResult.Success)?.data.orEmpty()
            _state.update { it.copy(sections = groupByMonth(photos), isLoading = false) }
        }
    }

    private fun groupByMonth(photos: List<Photo>): List<PhotoSection> =
        photos.sortedByDescending { it.takenAtMillis }
            .groupBy { yearMonthOf(it.takenAtMillis) }
            .toSortedMap(compareByDescending { it })
            .map { (ym, list) -> PhotoSection(title = monthFormat.format(ym), photos = list) }

    private fun yearMonthOf(millis: Long): YearMonth =
        YearMonth.from(Instant.ofEpochMilli(millis).atZone(zone).toLocalDate())

    fun thumbnailUrl(id: String): String = photoRepository.thumbnailUrl(id)
    fun previewUrl(id: String): String = photoRepository.previewUrl(id)
}
