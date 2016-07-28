#!/bin/bash

rm -rf bin
mkdir -p bin/release

for goos in linux darwin windows ; do
    for goarch in amd64 386; do
        # path
        outdir="bin/$goos/$goarch"
        path="$outdir/obj-simlify"
        if [ $goos = windows ] ; then
            path=$path.exe
        fi

        # build
        echo -e "\nBuilding $goos/$goarch"
        GOOS=$goos GOARCH=$goarch go build -o $path
        echo "  > `du -hc $path | awk 'NR==1{print $1;}'`    `file $path`"

        # compress (for unique filenames to github release files)
        if [ $goos = windows ] ; then
            zip -rjX ./bin/release/$goos-$goarch.zip ./$outdir/ > /dev/null 2>&1
        else
            tar -C ./$outdir/ -cvzf ./bin/release/$goos-$goarch.tar.gz . > /dev/null 2>&1
        fi
    done
done

echo -e "\nRelease done"
for goos in linux darwin windows ; do
    for goarch in amd64 386; do
        path=bin/release/$goos-$goarch.tar.gz
        if [ $goos = windows ] ; then
            path=bin/release/$goos-$goarch.zip
        fi
        echo "  > `du -hc $path | awk 'NR==1{print $1;}'`    $path"
    done
done

echo ""
