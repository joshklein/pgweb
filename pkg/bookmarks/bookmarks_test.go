package bookmarks

import (
	"testing"

	"github.com/sosedoff/pgweb/pkg/command"
	"github.com/sosedoff/pgweb/pkg/shared"
	"github.com/stretchr/testify/assert"
)

func Test_Invalid_Bookmark_Files(t *testing.T) {
	_, err := readServerConfig("foobar")
	assert.Error(t, err)

	_, err = readServerConfig("../../data/invalid.toml")
	assert.Error(t, err)
	assert.Equal(t, "toml: line 1: expected '.' or '=', but got 'e' instead", err.Error())
}

func Test_Bookmark(t *testing.T) {
	bookmark, err := readServerConfig("../../data/bookmark.toml")
	assert.Equal(t, nil, err)
	assert.Equal(t, "localhost", bookmark.Host)
	assert.Equal(t, 5432, bookmark.Port)
	assert.Equal(t, "postgres", bookmark.User)
	assert.Equal(t, "mydatabase", bookmark.Database)
	assert.Equal(t, "disable", bookmark.SSLMode)
	assert.Equal(t, "", bookmark.Password)
	assert.Equal(t, "", bookmark.URL)

	bookmark, err = readServerConfig("../../data/bookmark_invalid_ssl.toml")
	assert.Equal(t, nil, err)
	assert.Equal(t, "disable", bookmark.SSLMode)
}

func Test_Bookmark_URL(t *testing.T) {
	bookmark, err := readServerConfig("../../data/bookmark_url.toml")

	assert.Equal(t, nil, err)
	assert.Equal(t, "postgres://username:password@host:port/database?sslmode=disable", bookmark.URL)
	assert.Equal(t, "", bookmark.Host)
	assert.Equal(t, 5432, bookmark.Port)
	assert.Equal(t, "", bookmark.User)
	assert.Equal(t, "", bookmark.Database)
	assert.Equal(t, "disable", bookmark.SSLMode)
	assert.Equal(t, "", bookmark.Password)
}

func Test_Bookmarks_Path(t *testing.T) {
	assert.NotEqual(t, "/.pgweb/bookmarks", Path(""))
}

func Test_Basename(t *testing.T) {
	assert.Equal(t, "filename", fileBasename("filename.toml"))
	assert.Equal(t, "filename", fileBasename("path/filename.toml"))
	assert.Equal(t, "filename", fileBasename("~/long/path/filename.toml"))
	assert.Equal(t, "filename", fileBasename("filename"))
}

func Test_ReadBookmarks_Invalid(t *testing.T) {
	bookmarks, err := ReadAll("foobar")

	assert.Error(t, err)
	assert.Equal(t, 0, len(bookmarks))
}

func Test_ReadBookmarks(t *testing.T) {
	bookmarks, err := ReadAll("../../data")

	assert.Equal(t, nil, err)
	assert.Equal(t, 3, len(bookmarks))
}

func Test_GetBookmark(t *testing.T) {
	expBookmark := Bookmark{

		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "",
		Database: "mydatabase",
		SSLMode:  "disable",
	}
	b, err := GetBookmark("../../data", "bookmark")
	if assert.NoError(t, err) {
		assert.Equal(t, expBookmark, b)
	}

	_, err = GetBookmark("../../data", "bar")
	expErrStr := "couldn't find a bookmark with name bar"
	assert.Equal(t, expErrStr, err.Error())

	_, err = GetBookmark("foo", "bookmark")
	assert.Error(t, err)
}

func Test_Bookmark_SSHInfoIsEmpty(t *testing.T) {
	emptySSH := &shared.SSHInfo{
		Host: "",
		Port: "",
		User: "",
	}
	populatedSSH := &shared.SSHInfo{
		Host: "localhost",
		Port: "8080",
		User: "postgres",
	}

	b := Bookmark{SSH: nil}
	assert.True(t, b.SSHInfoIsEmpty())

	b = Bookmark{SSH: emptySSH}
	assert.True(t, b.SSHInfoIsEmpty())

	b.SSH = populatedSSH
	assert.False(t, b.SSHInfoIsEmpty())
}

func Test_ConvertToOptions(t *testing.T) {
	b := Bookmark{
		URL:      "postgres://username:password@host:port/database?sslmode=disable",
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "password",
		Database: "mydatabase",
		SSLMode:  "disable",
	}

	expOpt := command.Options{
		URL:     "postgres://username:password@host:port/database?sslmode=disable",
		Host:    "localhost",
		Port:    5432,
		User:    "postgres",
		Pass:    "password",
		DbName:  "mydatabase",
		SSLMode: "disable",
	}
	opt := b.ConvertToOptions()
	assert.Equal(t, expOpt, opt)
}
