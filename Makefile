build:
	mkdir -p dist
	go build -o dist/place.exe cmd/place/place.go 
	cp -r web dist/web
run-dist:
	cd ./dist && ./place.exe -root web/root -port :8080 -width 1280 -height 720 -log place.log $@
