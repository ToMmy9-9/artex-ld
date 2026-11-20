#!/usr/bin/env python3
"""
启动MitmProxy响应修改代理的便捷脚本
支持多种端口选择，避免与其他工具冲突
"""

import subprocess
import sys
import os

def get_available_ports():
    """返回可用的端口列表（排除用户已占用的端口）"""
    used_ports = [1080, 8080, 8083, 7897, 7890, 888]
    available_ports = [8090, 8091, 8092, 8093, 8094, 8095, 8096, 8097, 8098, 8099]
    return [p for p in available_ports if p not in used_ports]

def main():
    available_ports = get_available_ports()
    
    print("=" * 60)
    print("    MitmProxy 响应修改代理启动器")
    print("=" * 60)
    print(f"已排除占用端口: [1080, 8080, 8083, 7897, 7890, 888]")
    print()
    print("可用端口列表:")
    for i, port in enumerate(available_ports, 1):
        print(f"  {i}. 端口 {port}")
    print()
    
    # 默认选择第一个可用端口
    selected_port = available_ports[0]
    print(f"默认使用端口: {selected_port}")
    print()
    
    # 构建命令
    script_path = os.path.join(os.path.dirname(__file__), 'response_modifier.py')
    cmd = [
        'mitmproxy',
        '-s', script_path,
        '-p', str(selected_port)
    ]
    
    print(f"启动命令: {' '.join(cmd)}")
    print("=" * 60)
    print("按 Ctrl+C 停止代理")
    print("=" * 60)
    print()
    
    try:
        subprocess.run(cmd, check=True)
    except KeyboardInterrupt:
        print("\n代理已停止")
    except subprocess.CalledProcessError as e:
        print(f"代理启动失败: {e}")
        sys.exit(1)

if __name__ == '__main__':
    main()
