#!/bin/bash

git status | grep "master" &>/dev/null

if [[ $? -ne 0 ]]; then
    echo 'release should be on master branch!'
    exit
fi

function releaseTag() {
    git pull --tags
    i=0
    top=0
    second=0
    third=0
    while true; do
        local count=$(git tag -l "v$i.*" | wc -l | xargs printf '%d')
        if [[ ${count} -eq 0 ]]; then
            if [[ ${i} -eq 0 ]]; then
                top=0
                break
            fi
            top=`expr ${i} - 1`
            break
        fi
        i=`expr ${i} + 1`
    done
    i=0
    while true; do
        local count=$(git tag -l "v$top.$i.*" | wc -l | xargs printf '%d')
        if [[ ${count} -eq 0 ]]; then
            if [[ ${i} -eq 0 ]]; then
                second=0
                break
            fi
            second=`expr ${i} - 1`
            break
        fi
        i=`expr ${i} + 1`
    done
    third=$(git tag -l "v$top.$second.*" | wc -l | xargs printf '%d')
    if [[ ${third} -eq 0 ]]; then
        third=0
    else
        third=`expr ${third} - 1`
    fi
    local old_tag="v$top.$second.$third"

    local new_tag=""
    case $1 in
    top)
        top=`expr ${top} + 1`
        second=0
        third=0
       ;;
    second)
        second=`expr ${second} + 1`
        third=0
        ;;
    third)
        third=`expr ${third} + 1`
        ;;
    *)
        third=`expr ${third} + 1`
        ;;
    esac
    local new_tag="v$top.$second.$third"
    echo ${new_tag}
    git tag ${new_tag}
    git add .
    git commit -m "release-${new_tag}"
    git push
    git push origin ${new_tag}
}

releaseTag $1;
