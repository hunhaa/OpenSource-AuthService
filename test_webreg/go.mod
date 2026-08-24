module test_webreg

go 1.25.0

require github.com/Yeah114/g79client v0.0.0-00010101000000-000000000000

replace github.com/Yeah114/g79client => ../modules/g79client

require github.com/getcharzp/go-ocr v0.0.0-20260126073315-15e83dd6ccce // indirect

require (
	github.com/ebitengine/purego v0.9.0 // indirect
	github.com/getcharzp/onnxruntime_purego v0.0.0-20260118041137-401482b32507 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/up-zero/gotool v0.0.0-20260120011100-d685b2532b5a // indirect
)

replace github.com/getcharzp/go-ocr => ../modules/go-ocr
