// Package version storees the app's version information for use in diagnostics.
package version

//You can set these fields when building the binary using ldflags as well. For
//example, if you want to grab the version from a git tag.
//go build -ldflags="-X 'package_path.variable_name=new_value'"

// V is the version number of the app. This should match the git tag at the point this
// version was released. This value is stored here, and not in main.go, so that we can
// get it from any other package as needed (aka pages for diagnostic page).
const V = "1.1.3"

// ReleaseDate is the date this verison was released on.
const ReleaseDate = "2023-10-27"
