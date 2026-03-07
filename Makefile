BIN     := localsend
MAIN    := ./cmd/main.go
PREFIX  ?= $(HOME)/.local/bin

.PHONY: build install clean

build:
	go build -o $(BIN) $(MAIN)

install: build
	@mkdir -p $(PREFIX)
	cp $(BIN) $(PREFIX)/$(BIN)
	@echo "Installed to $(PREFIX)/$(BIN)"
	@echo "Make sure $(PREFIX) is in your PATH."

clean:
	rm -f $(BIN)
