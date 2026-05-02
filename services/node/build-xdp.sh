#!/bin/bash
# 编译 XDP eBPF 程序
# 需要在 Linux 环境下运行

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
XDP_DIR="$SCRIPT_DIR/xdp-ebpf"
OUTPUT_DIR="$SCRIPT_DIR/target/bpf"

echo "Building XDP eBPF program..."

# 检查是否安装了必要的工具
if ! command -v rustup &> /dev/null; then
    echo "Error: rustup is not installed"
    exit 1
fi

# 确保安装了 nightly 工具链和 rust-src
rustup toolchain install nightly --component rust-src

# 添加 BPF 目标
rustup target add bpfel-unknown-none --toolchain nightly 2>/dev/null || true

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

# 编译 eBPF 程序
cd "$XDP_DIR"
cargo +nightly build --release -Z build-std=core --target bpfel-unknown-none

# 复制编译结果
cp "$XDP_DIR/target/bpfel-unknown-none/release/lingcdn-xdp" "$OUTPUT_DIR/"

echo "XDP eBPF program built successfully: $OUTPUT_DIR/lingcdn-xdp"
