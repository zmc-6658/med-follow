"""MCP 客户端：asyncio 子进程拉起 Go 网关的 MCP Server，JSON-RPC 2.0 over stdio。

协议约定：stdin/stdout 每行一条 JSON；caller token 经环境变量 MEDFOLLOW_MCP_TOKEN
注入子进程，server 侧在 initialize 时校验。
AI 服务的一切数据获取只走 MCP，不直连数据库（与业务层解耦的关键点）。

P1 在 main.py lifespan 中接入（零改动）：
    #   mcp = MCPClient(server_binary=..., gateway_dir=..., token=...)
    #   await mcp.connect()
    #   yield
    #   await mcp.close()
"""
import asyncio
import json
import os
from pathlib import Path

import yaml

BASE_DIR = Path(__file__).resolve().parents[1]

_CODE_UNAUTHORIZED = -32001


class MCPError(Exception):
    """MCP 调用失败：JSON-RPC error 响应（code 为协议错误码）或工具执行错误。"""

    def __init__(self, message: str, code: int | None = None):
        super().__init__(message)
        self.code = code


def _default_settings() -> dict:
    cfg = yaml.safe_load((BASE_DIR / "etc" / "config.yaml").read_text(encoding="utf-8"))["mcp"]
    return {
        "server_binary": (BASE_DIR / cfg["server_binary"]).resolve(),
        "gateway_dir": (BASE_DIR / cfg["gateway_dir"]).resolve(),
        "token": cfg["token"],
    }


class MCPClient:
    """以子进程方式管理 Go MCP Server 生命周期；connect() = 拉起子进程 + initialize 握手。

    config_path 用于测试等场景覆盖 GATEWAY_CONFIG（默认走 gateway 目录的 etc/config.yaml）。
    """

    def __init__(self, server_binary: str | Path, gateway_dir: str | Path, token: str,
                 config_path: str | Path | None = None):
        self._server_binary = str(server_binary)
        self._gateway_dir = str(gateway_dir)
        self._token = token
        self._config_path = config_path
        self._proc: asyncio.subprocess.Process | None = None
        self._next_id = 0

    async def connect(self) -> "MCPClient":
        if self._proc is not None:
            raise RuntimeError("mcp client already connected")
        env = dict(os.environ, MEDFOLLOW_MCP_TOKEN=self._token)
        if self._config_path is not None:
            env["GATEWAY_CONFIG"] = str(self._config_path)
        self._proc = await asyncio.create_subprocess_exec(
            self._server_binary,
            cwd=self._gateway_dir,
            env=env,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
        )
        await self._request("initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {"name": "medfollow-ai-service", "version": "0.1.0"},
        })
        return self

    async def close(self):
        if self._proc is None:
            return
        proc, self._proc = self._proc, None
        try:
            await asyncio.wait_for(proc.wait(), timeout=3)
        except asyncio.TimeoutError:
            proc.kill()
            await proc.wait()

    async def list_tools(self) -> list[dict]:
        result = await self._request("tools/list")
        return result["tools"]

    async def call_tool(self, name: str, arguments: dict | None = None):
        result = await self._request("tools/call", {"name": name, "arguments": arguments or {}})
        if result.get("isError"):
            raise MCPError(result["content"][0]["text"])
        return json.loads(result["content"][0]["text"])

    async def _request(self, method: str, params: dict | None = None):
        if self._proc is None:
            raise MCPError("mcp client not connected")
        self._next_id += 1
        req: dict = {"jsonrpc": "2.0", "id": self._next_id, "method": method}
        if params is not None:
            req["params"] = params

        self._proc.stdin.write((json.dumps(req, ensure_ascii=False) + "\n").encode("utf-8"))
        await self._proc.stdin.drain()

        line = await self._proc.stdout.readline()
        if not line:
            raise MCPError("mcp server closed unexpectedly")
        resp = json.loads(line)
        if "error" in resp:
            err = resp["error"]
            raise MCPError(err.get("message", "unknown error"), err.get("code"))
        return resp.get("result")


# 供 P1 lifespan 直接使用的模块级单例（connect/close 语义见类注释）
mcp_client = MCPClient(**_default_settings())
