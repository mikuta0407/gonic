// author: spijet (https://github.com/spijet/)
// author: sentriz (https://github.com/sentriz/)

//nolint:gochecknoglobals
package transcode

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/google/shlex"
)

type TomlConfig struct {
	TranscodeProfile TomlTranscodeProfile
}

type TomlTranscodeProfile struct {
	Mime    string `toml:"mimetype"`
	Ext     string `toml:"ext"`
	Bitrate uint   `toml:"bitrate"`
	FFCmd   string `toml:"ffcmd"`
}

type Transcoder interface {
	Transcode(ctx context.Context, profile Profile, in string, out io.Writer) error
}

var UserProfiles = map[string]Profile{
	"mp3":          MP3,
	"mp3_rg":       MP3RG,
	"opus_car":     OpusRGLoud,
	"opus":         Opus,
	"opus_rg":      OpusRG,
	"opus_128_car": Opus128RGLoud,
	"opus_128":     Opus128,
	"opus_128_rg":  Opus128RG,
	"opus_192":     Opus192,
}

// Store as simple strings, since we may let the user provide their own profiles soon
var (
	MP3           Profile
	MP3RG         Profile
	Opus          Profile
	OpusRG        Profile
	OpusRGLoud    Profile
	Opus128       Profile
	Opus128RG     Profile
	Opus128RGLoud Profile
	Opus192       Profile
)

type BitRate uint // kilobits/s

type Profile struct {
	bitrate BitRate // the default bitrate, but the user can request a different one
	seek    time.Duration
	mime    string
	suffix  string
	exec    string
}

func (p *Profile) BitRate() BitRate    { return p.bitrate }
func (p *Profile) Seek() time.Duration { return p.seek }
func (p *Profile) Suffix() string      { return p.suffix }
func (p *Profile) MIME() string        { return p.mime }

func NewProfile(mime string, suffix string, bitrate BitRate, exec string) Profile {
	return Profile{mime: mime, suffix: suffix, bitrate: bitrate, exec: exec}
}

func WithBitrate(p Profile, bitRate BitRate) Profile {
	p.bitrate = bitRate
	return p
}

func WithSeek(p Profile, seek time.Duration) Profile {
	p.seek = seek
	return p
}

var ErrNoProfileParts = fmt.Errorf("not enough profile parts")

func parseProfile(profile Profile, in string) (string, []string, error) {
	parts, err := shlex.Split(profile.exec)
	if err != nil {
		return "", nil, fmt.Errorf("split command: %w", err)
	}
	if len(parts) == 0 {
		return "", nil, ErrNoProfileParts
	}
	name, err := exec.LookPath(parts[0])
	if err != nil {
		return "", nil, fmt.Errorf("find name: %w", err)
	}

	var args []string
	for _, p := range parts[1:] {
		switch p {
		case "<file>":
			args = append(args, in)
		case "<seek>":
			args = append(args, fmt.Sprintf("%dus", profile.Seek().Microseconds()))
		case "<bitrate>":
			args = append(args, fmt.Sprintf("%dk", profile.BitRate()))
		default:
			args = append(args, p)
		}
	}

	return name, args, nil
}
