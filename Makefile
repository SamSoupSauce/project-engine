ifeq ($(OS),Windows_NT)
  EXE_EXT := .exe
else
  EXE_EXT :=
endif

BIN_DIR := bin
DIST_DIR := dist/project-engine
TARGET := $(BIN_DIR)/project-engine$(EXE_EXT)
export CGO_LDFLAGS_ALLOW := -l:.*\.a

.PHONY: all build test vet run package clean

all: build

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(TARGET) ./cmd/demo
	@echo Built $(TARGET)

test:
	go test -count=1 ./...

vet:
	go vet ./...

run: build
	./$(TARGET)

package: build
	@mkdir -p $(DIST_DIR)
	@cp $(TARGET) $(DIST_DIR)/project-engine$(EXE_EXT)
	@cp engine.json $(DIST_DIR)/engine.json
	@cp -r assets $(DIST_DIR)/
	@cp -r scripts $(DIST_DIR)/
	@cp -r prefabs $(DIST_DIR)/
	@mkdir -p $(DIST_DIR)/config
	@echo Packaged standalone build at $(DIST_DIR)

clean:
	@rm -rf $(BIN_DIR) dist
