package prefetch

import "math"

// Session follows Silo's native /api/v2/admin/sessions response. Identity
// filters can name either an account or a household profile.
type Session struct {
	SessionID       string  `json:"session_id"`
	MediaType       string  `json:"media_type"`
	SeriesName      string  `json:"series_name"`
	SeasonNumber    *int    `json:"season_number"`
	EpisodeNumber   *int    `json:"episode_number"`
	UserID          string  `json:"user_id"`
	Username        string  `json:"username"`
	ProfileID       string  `json:"profile_id"`
	ProfileName     string  `json:"profile_name"`
	IsPaused        bool    `json:"is_paused"`
	PositionSeconds float64 `json:"position_seconds"`
	FileDuration    *int    `json:"file_duration"`
}

// Silo reports position and file duration in seconds. This is playback
// progress, including resumed/seeked positions, rather than elapsed watch time.
func (s Session) reachedProgress(percent int) bool {
	if percent == 0 {
		return true
	}
	if s.FileDuration == nil || *s.FileDuration <= 0 || s.PositionSeconds < 0 || math.IsNaN(s.PositionSeconds) || math.IsInf(s.PositionSeconds, 0) {
		return false
	}
	return s.PositionSeconds/float64(*s.FileDuration)*100 >= float64(percent)
}
