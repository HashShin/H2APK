package assets

import _ "embed"

//go:embed static/index.html
var IndexHTML string

//go:embed images/instapay.png
var InstapayPNG []byte

//go:embed images/binance.png
var BinancePNG []byte

//go:embed keystore/debug.keystore
var Keystore []byte
