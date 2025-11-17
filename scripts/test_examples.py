#!/usr/bin/env python3
"""
运行时测试所有 agentsdk-go 示例
"""

import subprocess
import sys
import time
import os
import json
from dataclasses import dataclass
from enum import Enum
from typing import Dict, List

class TestType(Enum):
    API = "api"          # 需要 API 调用的示例
    HTTP = "http"        # HTTP 服务示例
    QUICK = "quick"      # 快速本地示例
    DEPRECATED = "deprecated"  # 已废弃示例，不再执行

class TestResult(Enum):
    PASS = "✅ PASS"
    FAIL = "❌ FAIL"
    TIMEOUT = "⏱️  TIMEOUT"
    SKIP = "⏭️  SKIP"
    DEPRECATED = "⚠️  DEPRECATED"

@dataclass
class TestConfig:
    name: str
    test_type: TestType
    timeout: int
    expected_output: str = None

@dataclass
class TestReport:
    name: str
    result: TestResult
    output: str
    error: str = None
    duration: float = 0.0

# 测试配置
TEST_CONFIGS = [
    TestConfig("approval", TestType.QUICK, 10),
    TestConfig("basic", TestType.API, 30),
    TestConfig("checkpoint", TestType.API, 30),
    TestConfig("custom-tools", TestType.API, 30),
    TestConfig("http-full", TestType.HTTP, 10),
    TestConfig("http-simple", TestType.HTTP, 10),
    TestConfig("http-stream", TestType.HTTP, 10),
    TestConfig("mcp", TestType.QUICK, 10),
    TestConfig("model-openai", TestType.API, 30),
    TestConfig("security", TestType.QUICK, 10),
    TestConfig("simple-stream", TestType.API, 30),
    TestConfig("stream", TestType.DEPRECATED, 30),
    TestConfig("telemetry", TestType.API, 30),
    TestConfig("tool-basic", TestType.API, 30),
    TestConfig("tool-stream", TestType.API, 30),
    TestConfig("wal", TestType.QUICK, 10),
    TestConfig("workflow", TestType.QUICK, 10),
]

def load_env():
    """加载 .env 文件"""
    env_file = ".env"
    if os.path.exists(env_file):
        with open(env_file) as f:
            for line in f:
                line = line.strip()
                if line and not line.startswith('#') and '=' in line:
                    key, value = line.split('=', 1)
                    os.environ[key] = value.strip('"')
        print("✓ 已加载 .env 配置")
        api_key = os.getenv("ANTHROPIC_API_KEY", "")
        base_url = os.getenv("ANTHROPIC_BASE_URL", "")
        if api_key:
            print(f"  API Key: {api_key[:20]}...")
        if base_url:
            print(f"  Base URL: {base_url}")
        print()

def run_api_test(config: TestConfig) -> TestReport:
    """运行需要 API 的测试"""
    print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    print(f"测试: {config.name} (API)")
    print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

    start_time = time.time()
    try:
        result = subprocess.run(
            ["go", "run", f"./examples/{config.name}/"],  # 编译整个包
            capture_output=True,
            text=True,
            timeout=config.timeout,
            env=os.environ.copy()
        )
        duration = time.time() - start_time

        output = result.stdout + result.stderr
        print(output[:500] if len(output) > 500 else output)
        print()

        if result.returncode == 0:
            return TestReport(config.name, TestResult.PASS, output, duration=duration)
        else:
            error = f"Exit code: {result.returncode}"
            return TestReport(config.name, TestResult.FAIL, output, error, duration)

    except subprocess.TimeoutExpired:
        duration = time.time() - start_time
        print(f"⏱️  超时 ({config.timeout}s)\n")
        return TestReport(config.name, TestResult.TIMEOUT, "", f"超时 {config.timeout}s", duration)
    except Exception as e:
        duration = time.time() - start_time
        print(f"❌ 异常: {e}\n")
        return TestReport(config.name, TestResult.FAIL, "", str(e), duration)

def run_http_test(config: TestConfig) -> TestReport:
    """运行 HTTP 服务测试"""
    print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    print(f"测试: {config.name} (HTTP)")
    print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

    start_time = time.time()
    process = None
    try:
        # 启动服务
        process = subprocess.Popen(
            ["go", "run", f"./examples/{config.name}/"],  # 编译整个包
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            env=os.environ.copy()
        )

        # 等待服务启动
        time.sleep(3)

        # 测试健康检查
        try:
            result = subprocess.run(
                ["curl", "-s", "http://localhost:8080/health"],
                capture_output=True,
                timeout=5
            )
            if result.returncode == 0:
                duration = time.time() - start_time
                print("✓ HTTP 服务启动成功\n")
                return TestReport(config.name, TestResult.PASS, "HTTP service OK", duration=duration)
            else:
                duration = time.time() - start_time
                print("✗ 健康检查失败\n")
                return TestReport(config.name, TestResult.FAIL, "", "健康检查失败", duration)
        except Exception as e:
            duration = time.time() - start_time
            print(f"✗ 健康检查异常: {e}\n")
            return TestReport(config.name, TestResult.FAIL, "", str(e), duration)

    finally:
        if process:
            process.terminate()
            try:
                process.wait(timeout=2)
            except:
                process.kill()

def run_quick_test(config: TestConfig) -> TestReport:
    """运行快速本地测试"""
    print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    print(f"测试: {config.name} (本地)")
    print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

    start_time = time.time()
    try:
        result = subprocess.run(
            ["go", "run", f"./examples/{config.name}/"],  # 编译整个包
            capture_output=True,
            text=True,
            timeout=config.timeout,
            env=os.environ.copy()
        )
        duration = time.time() - start_time

        output = result.stdout + result.stderr
        print(output[:500] if len(output) > 500 else output)
        print()

        if result.returncode == 0:
            return TestReport(config.name, TestResult.PASS, output, duration=duration)
        else:
            error = f"Exit code: {result.returncode}"
            return TestReport(config.name, TestResult.FAIL, output, error, duration)

    except subprocess.TimeoutExpired:
        duration = time.time() - start_time
        print(f"⏱️  超时 ({config.timeout}s)\n")
        return TestReport(config.name, TestResult.TIMEOUT, "", f"超时 {config.timeout}s", duration)
    except Exception as e:
        duration = time.time() - start_time
        print(f"❌ 异常: {e}\n")
        return TestReport(config.name, TestResult.FAIL, "", str(e), duration)

def run_test(config: TestConfig) -> TestReport:
    """根据测试类型运行相应的测试"""
    if config.test_type == TestType.API:
        return run_api_test(config)
    elif config.test_type == TestType.HTTP:
        return run_http_test(config)
    elif config.test_type == TestType.QUICK:
        return run_quick_test(config)
    elif config.test_type == TestType.DEPRECATED:
        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
        print(f"测试: {config.name} (已废弃，跳过)")
        print(f"━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
        print("⚠️  示例已废弃，未执行。\n")
        return TestReport(config.name, TestResult.DEPRECATED, "示例已废弃，未执行")
    else:
        return TestReport(config.name, TestResult.FAIL, "", "未知测试类型")

def print_summary(reports: List[TestReport]):
    """打印测试摘要"""
    print("=" * 60)
    print("  测试报告")
    print("=" * 60)
    print()

    # 按结果分组
    passed = [r for r in reports if r.result == TestResult.PASS]
    failed = [r for r in reports if r.result == TestResult.FAIL]
    timeout = [r for r in reports if r.result == TestResult.TIMEOUT]
    skipped = [r for r in reports if r.result == TestResult.SKIP]
    deprecated = [r for r in reports if r.result == TestResult.DEPRECATED]

    # 打印所有结果
    for report in sorted(reports, key=lambda x: x.name):
        duration_str = f"({report.duration:.2f}s)" if report.duration > 0 else ""
        print(f"{report.result.value:12s} {report.name:20s} {duration_str}")
        if report.error:
            print(f"             错误: {report.error}")

    print()
    print("=" * 60)
    print("  统计")
    print("=" * 60)
    print(f"总数: {len(reports)}")
    print(f"通过: {len(passed)}")
    print(f"失败: {len(failed)}")
    print(f"超时: {len(timeout)}")
    print(f"跳过: {len(skipped)}")
    print(f"已废弃: {len(deprecated)}")
    print()

    # 保存 JSON 报告
    report_data = {
        "total": len(reports),
        "passed": len(passed),
        "failed": len(failed),
        "timeout": len(timeout),
        "skipped": len(skipped),
        "deprecated": len(deprecated),
        "tests": [
            {
                "name": r.name,
                "result": r.result.value,
                "duration": r.duration,
                "error": r.error
            }
            for r in reports
        ]
    }

    with open("test_report.json", "w") as f:
        json.dump(report_data, f, indent=2)
    print("✓ 测试报告已保存到 test_report.json")
    print()

    if len(failed) == 0 and len(timeout) == 0:
        print("✅ 所有测试通过！")
        return 0
    else:
        print(f"❌ 有 {len(failed) + len(timeout)} 个测试失败或超时")
        return 1

def main():
    print("=" * 60)
    print("  运行时测试所有示例")
    print("=" * 60)
    print()

    # 加载环境变量
    load_env()

    # 运行所有测试
    reports = []
    for config in TEST_CONFIGS:
        report = run_test(config)
        reports.append(report)

    # 打印摘要
    exit_code = print_summary(reports)
    sys.exit(exit_code)

if __name__ == "__main__":
    main()
