# Todo List

An exploration of Go templates and HTMLX.

This is a functional prototype. It is not, nor does it pretend to be, production-ready code.

Huge hat tip to the [Golang app skeleton](https://adhoc.team/2021/03/29/simple-web-app-in-golang/) provided by the folks at [Ad Hoc](https://adhoc.team) that made the app server code so much more legible than my first pass, but still kept things dead simple and easy to develop against. It was a nice piece of kit for a simple exploration like this.

## Pre-requisites

* Go 1.19 or better
* SQLite3
* [direnv](https://direnv.net)

## Installation

1. Clone this repo
2. Install the environment variables; this cannot be done from the install script because the script has no access to the operating shell environment (this trips me up more often than I'd like to admit)

        cp .envrc.example .envrc && direnv allow .

3. Run the install script from the project root

        bash ./install
