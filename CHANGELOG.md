# Changelog

## v0.0.151 — 2026-10-10

### Changes
- [afcae37](https://github.com/achandrapaul/digest/commit/afcae37f7d8ed00fa4f48afad5199e12ef146e7b) fix: streak ignores the zone notes were saved in
  - habit.Streak keys finished days by the day in the current zone, so notes read back as UTC or saved under another offset still count
  - the migrate dry-run test compares first_note_created by instant instead of by location
