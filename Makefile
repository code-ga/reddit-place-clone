LOAD_IMAGE_FLAG=
SAVE_IMAGE_FLAG=

ifdef LOAD_IMAGE
	LOAD_IMAGE_FLAG=-load $(LOAD_IMAGE)
endif

ifdef SAVE_IMAGE
	SAVE_IMAGE_FLAG=-load $(SVAE_IMAGE)
endif

build:
	mkdir -p dist
	go build -o dist/place.exe cmd/place/place.go 
	cp -r web dist/web
run-dist:
	cd ./dist && ./place.exe -root web/root -port :8080 -width 1280 -height 720 -log place.log $(LOAD_IMAGE_FLAG) $(SAVE_IMAGE_FLAG)
