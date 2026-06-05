#!/bin/bash

# 函数：计算剩余天数
calculate_days_left() {
    local expire_date="$1"
    local current_epoch=$(date +%s)
    local expire_epoch=$(date --date="$expire_date" +%s 2>/dev/null)

    if [ -z "$expire_epoch" ]; then
        echo "INVALID"
        return
    fi

    if [ $expire_epoch -gt $current_epoch ]; then
        local seconds_left=$((expire_epoch - current_epoch))
        local days_left=$((seconds_left / 86400))
        echo "$days_left"
    else
        echo "EXPIRED"
    fi
}

# 函数：检查证书信息并格式化输出
check_cert_info() {
    local dir="$1"
    local description="$2"

    printf '\n%s\n' "$description"
    printf '%-12s %-12s %-10s %-50s %s\n' "start_time" "expire_time" "days_left" "subject" "ssl_path_name"
    printf '%.12s %.12s %.10s %.50s %s\n' "------------" "------------" "----------" "--------------------------------------------------" "--------------------------------------------------"

    local CRT_LIST
    if [[ "$dir" == *"maxdepth"* ]]; then
        CRT_LIST=$(eval "find $dir -iname '*.crt' -o -iname '*.pem' 2>/dev/null" | grep -v '\-key\.')
    else
        CRT_LIST=$(find "$dir" \( -iname "*.crt" -o -iname "*.pem" \) 2>/dev/null | grep -v '\-key\.')
    fi

    if [ -z "$CRT_LIST" ]; then
        printf '%-12s %-12s %-10s %-50s %s\n' "N/A" "N/A" "N/A" "N/A" "No certificates found"
        return
    fi

    {
    for crt in $CRT_LIST; do
        if [ -f "$crt" ]; then
            # 获取证书信息
            start_time=$(openssl x509 -startdate -noout -in "$crt" 2>/dev/null | cut -d= -f 2-)
            expire_time=$(openssl x509 -enddate -noout -in "$crt" 2>/dev/null | cut -d= -f 2-)
            subject=$(openssl x509 -subject -noout -in "$crt" 2>/dev/null | sed 's/subject=//' | sed 's/^[ \t]*//;s/[ \t]*$//')

            if [ -n "$start_time" ] && [ -n "$expire_time" ] && [ -n "$subject" ]; then
                start_iso=$(date --date="$start_time" "+%Y-%m-%d" 2>/dev/null || echo "Invalid Date")
                expire_iso=$(date --date="$expire_time" "+%Y-%m-%d" 2>/dev/null || echo "Invalid Date")
                days_left=$(calculate_days_left "$expire_time")

                printf '%-12s %-12s %-10s %-50s %s\n' "$start_iso" "$expire_iso" "$days_left" "$subject" "$crt"
            else
                printf '%-12s %-12s %-10s %-50s %s\n' "ERROR" "ERROR" "ERROR" "ERROR" "$crt"
            fi
        fi
    done
    } | sort -k2  # 按过期时间排序
}

# 检查服务器证书
check_cert_info "/var/lib/rancher/rke2/server/tls/" "Server TLS Certificates:"

# 检查代理证书
check_cert_info "/var/lib/rancher/rke2/agent/ -maxdepth 1" "Agent Certificates:"
