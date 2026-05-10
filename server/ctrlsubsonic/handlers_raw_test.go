package ctrlsubsonic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.senan.xyz/gonic/db"
	"go.senan.xyz/gonic/mockfs"
	"go.senan.xyz/gonic/playlist"
	"go.senan.xyz/gonic/server/ctrlsubsonic/specidpaths"
	"go.senan.xyz/gonic/transcode"
)

func TestCoverForPlaylist(t *testing.T) {
	t.Parallel()

	t.Run("cover found jpg", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		store, err := playlist.NewStore(tmp)
		require.NoError(t, err)

		playlistDir := filepath.Join(tmp, "1")
		require.NoError(t, os.MkdirAll(playlistDir, 0o755))
		require.NoError(t, touch(filepath.Join(playlistDir, "test-playlist.jpg")))

		playlistID := playlistIDEncode("1/test-playlist.m3u")
		file, err := coverForPlaylist(store, playlistID)
		require.NoError(t, err)
		require.NotNil(t, file)
		require.NoError(t, file.Close())
	})

	t.Run("cover found png", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		store, err := playlist.NewStore(tmp)
		require.NoError(t, err)

		playlistDir := filepath.Join(tmp, "1")
		require.NoError(t, os.MkdirAll(playlistDir, 0o755))
		require.NoError(t, touch(filepath.Join(playlistDir, "test-playlist.png")))

		playlistID := playlistIDEncode("1/test-playlist.m3u")
		file, err := coverForPlaylist(store, playlistID)
		require.NoError(t, err)
		require.NotNil(t, file)
		require.NoError(t, file.Close())
	})

	t.Run("cover not found", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		store, err := playlist.NewStore(tmp)
		require.NoError(t, err)

		playlistDir := filepath.Join(tmp, "1")
		require.NoError(t, os.MkdirAll(playlistDir, 0o755))

		playlistID := playlistIDEncode("1/test-playlist.m3u")
		file, err := coverForPlaylist(store, playlistID)
		require.ErrorIs(t, err, errCoverEmpty)
		require.Nil(t, file)
	})

	t.Run("nested playlist path", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		store, err := playlist.NewStore(tmp)
		require.NoError(t, err)

		playlistDir := filepath.Join(tmp, "1", "subfolder")
		require.NoError(t, os.MkdirAll(playlistDir, 0o755))
		require.NoError(t, touch(filepath.Join(playlistDir, "my-nested-playlist.jpg")))

		playlistID := playlistIDEncode("1/subfolder/my-nested-playlist.m3u")
		file, err := coverForPlaylist(store, playlistID)
		require.NoError(t, err)
		require.NotNil(t, file)
		require.NoError(t, file.Close())
	})

	t.Run("playlist directory missing", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		store, err := playlist.NewStore(tmp)
		require.NoError(t, err)

		playlistID := playlistIDEncode("1/test-playlist.m3u")
		file, err := coverForPlaylist(store, playlistID)
		require.Error(t, err)
		require.Nil(t, file)
	})

	t.Run("different playlists different covers", func(t *testing.T) {
		t.Parallel()

		tmp := t.TempDir()
		store, err := playlist.NewStore(tmp)
		require.NoError(t, err)

		playlistDir := filepath.Join(tmp, "1")
		require.NoError(t, os.MkdirAll(playlistDir, 0o755))
		require.NoError(t, touch(filepath.Join(playlistDir, "playlist-a.jpg")))
		require.NoError(t, touch(filepath.Join(playlistDir, "playlist-b.png")))

		fileA, err := coverForPlaylist(store, playlistIDEncode("1/playlist-a.m3u"))
		require.NoError(t, err)
		require.Equal(t, "playlist-a.jpg", filepath.Base(fileA.Name()))
		require.NoError(t, fileA.Close())

		fileB, err := coverForPlaylist(store, playlistIDEncode("1/playlist-b.m3u"))
		require.NoError(t, err)
		require.Equal(t, "playlist-b.png", filepath.Base(fileB.Name()))
		require.NoError(t, fileB.Close())
	})
}

func TestServeStreamRawFormatDoesNotTranscode(t *testing.T) {
	t.Parallel()

	controller, transcoder, user, trackID := makeStreamController(t)
	require.NoError(t, controller.dbc.Create(&db.TranscodePreference{UserID: user.ID, Client: mockClientName, Profile: "mp3"}).Error)

	rr, req := makeStreamHTTPMock(user, "/stream", url.Values{
		"id":     []string{trackID},
		"format": []string{"raw"},
	})

	require.Nil(t, controller.ServeStream(rr, req))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Zero(t, transcoder.calls)
	require.NotZero(t, rr.Body.Len())
}

func TestServeDownloadDoesNotTranscode(t *testing.T) {
	t.Parallel()

	controller, transcoder, user, trackID := makeStreamController(t)
	require.NoError(t, controller.dbc.Create(&db.TranscodePreference{UserID: user.ID, Client: mockClientName, Profile: "mp3"}).Error)

	rr, req := makeStreamHTTPMock(user, "/download", url.Values{
		"id": []string{trackID},
	})

	require.Nil(t, controller.ServeDownload(rr, req))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Zero(t, transcoder.calls)
	require.NotZero(t, rr.Body.Len())
}

func TestServeStreamUsesRequestedFormatBitRate(t *testing.T) {
	t.Parallel()

	controller, transcoder, user, trackID := makeStreamController(t)

	rr, req := makeStreamHTTPMock(user, "/stream", url.Values{
		"id":      []string{trackID},
		"format":  []string{"mp3"},
		"bitRate": []string{"192"},
	})

	require.Nil(t, controller.ServeStream(rr, req))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, 1, transcoder.calls)
	require.Equal(t, transcode.BitRate(192), transcoder.profile.BitRate())
	require.Equal(t, "audio/mpeg", rr.Header().Get("Content-Type"))
	require.Equal(t, "transcoded", rr.Body.String())
}

func TestServeStreamUsesRequestedBitRateWithPreference(t *testing.T) {
	t.Parallel()

	controller, transcoder, user, trackID := makeStreamController(t)
	require.NoError(t, controller.dbc.Create(&db.TranscodePreference{UserID: user.ID, Client: mockClientName, Profile: "mp3"}).Error)

	rr, req := makeStreamHTTPMock(user, "/stream", url.Values{
		"id":         []string{trackID},
		"maxBitRate": []string{"192"},
	})

	require.Nil(t, controller.ServeStream(rr, req))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, 1, transcoder.calls)
	require.Equal(t, transcode.BitRate(192), transcoder.profile.BitRate())
	require.Equal(t, "audio/mpeg", rr.Header().Get("Content-Type"))
}

func TestServeStreamUsesDefaultMP3WhenBitRateRequestedWithoutPreference(t *testing.T) {
	t.Parallel()

	controller, transcoder, user, trackID := makeStreamControllerWithBitrate(t, 500)

	rr, req := makeStreamHTTPMock(user, "/stream", url.Values{
		"id":         []string{trackID},
		"maxBitRate": []string{"320"},
	})

	require.Nil(t, controller.ServeStream(rr, req))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, 1, transcoder.calls)
	require.Equal(t, transcode.BitRate(320), transcoder.profile.BitRate())
	require.Equal(t, "audio/mpeg", rr.Header().Get("Content-Type"))
}

type spyTranscoder struct {
	calls   int
	profile transcode.Profile
	in      string
}

func (s *spyTranscoder) Transcode(_ context.Context, profile transcode.Profile, in string, out io.Writer) error {
	s.calls++
	s.profile = profile
	s.in = in
	_, err := io.WriteString(out, "transcoded")
	return err
}

func makeStreamController(t *testing.T) (*Controller, *spyTranscoder, *db.User, string) {
	t.Helper()
	return makeStreamControllerWithBitrate(t, 320)
}

func makeStreamControllerWithBitrate(t *testing.T, bitrate uint) (*Controller, *spyTranscoder, *db.User, string) {
	t.Helper()

	m := mockfs.NewWithDirs(t, []string{""})
	m.AddItemsPrefixWithCovers("")
	m.SetAudio("artist-0/album-0/track-0.flac", 10*time.Second, bitrate, audioPath10s)
	m.ScanAndClean()
	m.ResetDates()

	transcoder := &spyTranscoder{}
	controller := &Controller{
		dbc:        m.DB(),
		musicPaths: []MusicPath{{Path: m.TmpDir()}},
		transcoder: transcoder,
	}

	user := controller.dbc.GetUserByName(mockUsername)
	require.NotNil(t, user)

	id, err := specidpaths.Lookup(
		controller.dbc,
		MusicPaths(controller.musicPaths),
		filepath.Join(m.TmpDir(), "podcasts"),
		filepath.Join(m.TmpDir(), "artist-0/album-0/track-0.flac"),
	)
	require.NoError(t, err)

	return controller, transcoder, user, id.String()
}

func makeStreamHTTPMock(user *db.User, path string, query url.Values) (*httptest.ResponseRecorder, *http.Request) {
	rr, req := makeHTTPMock(query)
	req.URL.Path = path
	ctx := context.WithValue(req.Context(), CtxUser, user)
	req = req.WithContext(ctx)
	return rr, req
}

func touch(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
