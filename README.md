![Alt text](images/EepTorrentLogo.png)

## ⚠️ Disclaimer ⚠️ (PLEASE READ BEFORE USING)
This is a work in progress and is considered **pre-alpha** software, meaning it doesn't even have the core functions yet. Use at your own risk and be sure to update frequently.

### As of 11/28/2024, this is in a semi-functional state. It's advised NOT to use at this time. But you can out of curiosity. Please check back soon.

## Building

```shell
git clone https://github.com/hkh4n/EepTorrent.git
cd EepTorrent
go mod tidy
make build-native
```

## Testing

Set /tmp/test.torrent to a valid torrent file to test out the trackers.

## Core libraries used

go-i2p-bt -> https://github.com/go-i2p/go-i2p-bt (fork of https://github.com/xgfone/go-bt)

sam3 -> https://github.com/go-i2p/sam3

i2pkeys -> https://github.com/go-i2p/i2pkeys

## Trademark and Logo Usage

The EepTorrent logos, brand names, and trademarks are not licensed under the same terms as the code. All rights reserved Haris Khan © 2024.

The logos and brand assets in `/images` are proprietary and may not be used without express written permission from Haris Khan.