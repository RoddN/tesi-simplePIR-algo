module tesi-simplepir

go 1.22

toolchain go1.22.2

require (
	github.com/ahenzinger/simplepir v0.0.0
	github.com/ugorji/go/codec v1.3.2
)

replace github.com/ahenzinger/simplepir => ./simplepir
