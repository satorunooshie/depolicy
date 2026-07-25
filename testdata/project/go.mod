module github.com/satorunooshie/depolicytest

go 1.26.0

require example.com/ext v0.0.0

replace example.com/ext => ./third_party/ext
