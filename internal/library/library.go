// Package library is where the app's music comes from: the listener, their
// playlists and liked songs, albums, artists and searches. Library is what
// the app asks; Spotify's Web API answers for it, and Fake in tests and in
// development.
package library

import (
	"errors"

	"mygoify/internal/music"
)

// Library answers for the listener's music. Its methods block, and are
// called on goroutines of their own.
type Library interface {
	// Me is the listener signed in.
	Me() (*music.User, error)
	// Home is what the listener played lately and listens to most.
	Home() (music.Home, error)
	// MyPlaylists are the playlists the listener made, their liked songs
	// first, and those they saved.
	MyPlaylists() (made, saved []music.Playlist, err error)
	// Playlist returns a playlist with its tracks, unless it is Hidden.
	Playlist(id string) (*music.Playlist, error)
	// Album returns an album with its songs.
	Album(id string) (*music.AlbumProfile, error)
	// Artist returns an artist with their most popular songs.
	Artist(id string) (*music.ArtistProfile, error)
	// ArtistAlbums returns limit of an artist's albums from offset, and
	// whether more follow.
	ArtistAlbums(id string, limit, offset int) ([]music.AlbumProfile, bool, error)
	// Search returns limit of what matches query, of a kind, from offset.
	Search(query string, kind music.SearchType, limit, offset int) (music.SearchResult, error)

	// LikedIDs are the tracks the listener likes.
	LikedIDs() ([]string, error)
	Like(trackID string, like bool) error
	FollowArtist(id string, follow bool) error

	CreatePlaylist(name string) (*music.Playlist, error)
	RenamePlaylist(id, name string) error
	AddToPlaylist(id string, trackIDs ...string) error
	RemoveFromPlaylist(id string, trackIDs ...string) error
	// DeletePlaylist deletes one of the listener's playlists: Spotify
	// unfollows it, as deleting one does in its apps.
	DeletePlaylist(id string) error
}

// ErrInPlaylist is what AddToPlaylist returns when the track is there
// already. Spotify would add it again: the library checks first.
var ErrInPlaylist = errors.New("the track is in the playlist already")
