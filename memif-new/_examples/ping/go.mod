module ping

go 1.16

require (
	github.com/google/btree v1.0.1 // indirect
	github.com/google/netstack v0.0.0-20191123085552-55fcc16cd0eb
	github.com/pkg/profile v1.6.0
	memif v0.0.0
)

replace memif v0.0.0 => ../../memif
