/*
Copyright 2016 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package translate

import (
	"fmt"
	"strings"

	"github.com/cloudfoundry-attic/jibber_jabber"
	"github.com/golang/glog"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml"
	"golang.org/x/text/language"
)

var (
	// preferredLanguage is the default language messages will be output in
	preferredLanguage = language.AmericanEnglish
	// our default language
	defaultLanguage = language.AmericanEnglish

	localizer *i18n.Localizer
)

// Translate translates the given string to the supplied langauge.
func Translate(m i18n.Message) string {

	/*if preferredLanguage == defaultLanguage {
		return m.Other
	}*/

	s, err := localizer.Localize(&i18n.LocalizeConfig{DefaultMessage: &m})
	fmt.Printf("LOCALIZED STRING = %s", s)
	if err != nil {
		fmt.Println(err)
		glog.Warningf("Failed to translate %s: %s", m.Other, err)
		return m.Other
	}
	return s
}

// DetermineLocale finds the system locale and sets the preferred language
// for output appropriately.
func DetermineLocale() {
	locale, err := jibber_jabber.DetectIETF()
	if err != nil {
		glog.Warningf("Getting system locale failed: %s", err)
		locale = ""
	}
	SetPreferredLanguage(locale)

	if preferredLanguage != defaultLanguage {
		bundle := i18n.NewBundle(defaultLanguage)
		bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
		/*filepath.Walk("translations", func(path string, info os.FileInfo, err error) error {
			bundle.LoadMessageFile(path)
			return nil
		})*/
		bundle.LoadMessageFile("active.fr.toml")
		localizer = i18n.NewLocalizer(bundle, locale)
	}
}

// SetPreferredLanguageTag configures which language future messages should use.
func SetPreferredLanguageTag(l language.Tag) {
	glog.Infof("Setting Language to %s ...", l)
	preferredLanguage = l
}

// SetPreferredLanguage configures which language future messages should use, based on a LANG string.
func SetPreferredLanguage(s string) error {
	// "C" is commonly used to denote a neutral POSIX locale. See http://pubs.opengroup.org/onlinepubs/009695399/basedefs/xbd_chap07.html#tag_07_02
	if s == "" || s == "C" {
		SetPreferredLanguageTag(defaultLanguage)
		return nil
	}
	// Handles "de_DE" or "de_DE.utf8"
	// We don't process encodings, since Rob Pike invented utf8 and we're mostly stuck with it.
	parts := strings.Split(s, ".")
	l, err := language.Parse(parts[0])
	if err != nil {
		return err
	}
	SetPreferredLanguageTag(l)
	return nil
}

// GetPreferredLanguage returns the preferred language tag.
func GetPreferredLanguage() language.Tag {
	return preferredLanguage
}
