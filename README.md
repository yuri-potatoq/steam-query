# Steam Query

### Usage:

#### Dev
```bash
# default output path is current directory
go run . -game-url <game-url>

OUTPUT_DIR="/home/user/Downloads" go run . -game-url <game-url>
```

#### Docker
```bash
docker build -f Dockerfile -t steam-query .
docker run -it -v ./output:/app/output steam-query -game-page <game-url>
```

### TODO:
- [ ] Build packages to distribute for most commum platforms

### Refs:
- https://ffmpeg.org/doxygen/trunk/index.html
- nix bundle --bundler github:NixOS/bundlers#toDEB .#default