#!/usr/bin/env python3
"""
Gastown MCP Server

Simple MCP endpoint for communicating with the Gastown Mayor.
The Mayor is a Claude Code agent that handles all task orchestration internally.

Single Tool:
  - mayor_task: Send a task to the Mayor, get response

The Mayor handles everything else internally (rigs, convoys, polecats, etc.)

Execution Modes:
  - Interactive (preferred): Uses `gt mayor chat` when Mayor session is running
    - Preserves conversation context
    - Lower latency (no Claude startup)
    - Stateful conversations
  - One-shot (fallback): Uses `claude -p` when Mayor session is not running
    - Fresh context each call
    - Stateless
"""

import asyncio
import hashlib
import hmac
import json
import logging
import os
import sys
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Dict, Optional

from aiohttp import web

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s',
    handlers=[logging.StreamHandler(sys.stdout)]
)
logger = logging.getLogger(__name__)

# Configuration from environment variables
GT_ROOT = os.environ.get('GT_ROOT', '/home/gastown/gt')
INSTANCE_TOKEN = os.environ.get('INSTANCE_TOKEN', '')
MCP_PORT = int(os.environ.get('MCP_PORT', '8081'))
MCP_HOST = os.environ.get('MCP_HOST', '0.0.0.0')
MCP_IDENTITY = os.environ.get('MCP_IDENTITY', 'overseer')
INSTANCE_TOKEN_FILE = os.environ.get('INSTANCE_TOKEN_FILE', '/tmp/gastown/instance_token')

# Timeout configuration (seconds)
TIMESTAMP_FRESHNESS = int(os.environ.get('TIMESTAMP_FRESHNESS', '300'))
MAYOR_STATUS_TIMEOUT = int(os.environ.get('MAYOR_STATUS_TIMEOUT', '5'))
MAIL_INBOX_TIMEOUT = int(os.environ.get('MAIL_INBOX_TIMEOUT', '10'))
POLL_INTERVAL = int(os.environ.get('POLL_INTERVAL', '2'))
NUDGE_TIMEOUT = int(os.environ.get('NUDGE_TIMEOUT', '30'))
DEFAULT_TASK_TIMEOUT = int(os.environ.get('DEFAULT_TASK_TIMEOUT', '300'))
MAX_TASK_TIMEOUT = int(os.environ.get('MAX_TASK_TIMEOUT', '600'))

# Server metadata
SERVER_VERSION = os.environ.get('SERVER_VERSION', '1.0.0')
PROTOCOL_VERSION = os.environ.get('PROTOCOL_VERSION', '2024-11-05')

# In-memory task tracking
@dataclass
class TaskRecord:
    task_id: str
    prompt: str
    status: str = "pending"  # pending, running, completed, failed
    result: Optional[str] = None
    error: Optional[str] = None
    created_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    completed_at: Optional[datetime] = None

TASKS: Dict[str, TaskRecord] = {}


# Single MCP Tool - the Mayor handles everything else
TOOLS = [
    {
        "name": "mayor_task",
        "description": "Send a task to the Gastown Mayor. The Mayor is a Claude Code agent that orchestrates work across multiple AI agents, repositories (rigs), and work streams (convoys). Just describe what you want done - the Mayor handles the rest. When the Mayor session is running (interactive mode), conversations preserve context. When stopped, falls back to stateless one-shot execution.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "task": {
                    "type": "string",
                    "description": "What you want the Mayor to do. Can be anything: 'add user auth to the app', 'fix the bug in checkout', 'list all active work', 'what's the status of the frontend rig?', etc."
                },
                "context": {
                    "type": "string",
                    "description": "Optional additional context (e.g., error messages, requirements, preferences)"
                },
                "wait": {
                    "type": "boolean",
                    "description": "If true, wait for task completion (up to timeout). If false, returns immediately with task_id for polling. Default: true"
                },
                "timeout": {
                    "type": "integer",
                    "description": f"Timeout in seconds when wait=true (default: {DEFAULT_TASK_TIMEOUT}, max: {MAX_TASK_TIMEOUT})"
                }
            },
            "required": ["task"]
        }
    }
]


def verify_signature(body: str, signature: str, timestamp: str) -> bool:
    """Verify HMAC signature from Conducktor."""
    if not INSTANCE_TOKEN:
        # No token configured, skip verification (dev mode)
        logger.warning("No INSTANCE_TOKEN configured, skipping signature verification")
        return True

    # Check timestamp freshness
    try:
        ts = int(timestamp)
        now = int(time.time())
        if abs(now - ts) > TIMESTAMP_FRESHNESS:
            logger.warning(f"Timestamp too old: {ts} vs {now}")
            return False
    except (ValueError, TypeError):
        logger.warning(f"Invalid timestamp: {timestamp}")
        return False

    # Compute expected signature
    signed_payload = f"{timestamp}.{body}"
    expected = hmac.new(
        INSTANCE_TOKEN.encode(),
        signed_payload.encode(),
        hashlib.sha256
    ).hexdigest()

    return hmac.compare_digest(signature, expected)


async def check_mayor_session_running() -> bool:
    """Check if the Mayor tmux session is active."""
    try:
        proc = await asyncio.create_subprocess_exec(
            'gt', 'mayor', 'status',
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=GT_ROOT
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=MAYOR_STATUS_TIMEOUT)
        output = stdout.decode().lower()
        # gt mayor status returns "running" or similar if active
        return proc.returncode == 0 and 'running' in output
    except Exception as e:
        logger.debug(f"Mayor status check failed: {e}")
        return False


async def clear_overseer_inbox() -> None:
    """Clear existing messages from overseer inbox before sending new task."""
    try:
        # Get all messages in overseer inbox
        proc = await asyncio.create_subprocess_exec(
            'gt', 'mail', 'inbox',
            '--identity', MCP_IDENTITY,
            '--json',
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=GT_ROOT
        )
        stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=MAIL_INBOX_TIMEOUT)
        output = stdout.decode().strip()

        if output and output != 'null':
            messages = json.loads(output)
            for msg in messages:
                msg_id = msg.get('id')
                if msg_id:
                    # Archive each message
                    archive_proc = await asyncio.create_subprocess_exec(
                        'gt', 'mail', 'archive', msg_id,
                        stdout=asyncio.subprocess.PIPE,
                        stderr=asyncio.subprocess.PIPE,
                        cwd=GT_ROOT
                    )
                    await archive_proc.communicate()
    except Exception as e:
        logger.debug(f"Failed to clear overseer inbox: {e}")


async def poll_for_reply(task_id: str, thread_id: str, timeout: int) -> Optional[Dict[str, Any]]:
    """
    Poll the overseer inbox for a reply from the Mayor.

    Returns the reply message or None if timeout.
    """
    elapsed = 0

    while elapsed < timeout:
        try:
            proc = await asyncio.create_subprocess_exec(
                'gt', 'mail', 'inbox',
                '--identity', MCP_IDENTITY,
                '--json',
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=GT_ROOT
            )
            stdout, _ = await asyncio.wait_for(proc.communicate(), timeout=MAIL_INBOX_TIMEOUT)
            output = stdout.decode().strip()

            if output and output != 'null':
                messages = json.loads(output)
                for msg in messages:
                    # Look for a reply in the same thread or from mayor
                    msg_from = msg.get('from', '').lower()
                    msg_thread = msg.get('thread_id', '')
                    msg_type = msg.get('type', '')

                    # Accept replies from mayor or in the same thread
                    if 'mayor' in msg_from or msg_thread == thread_id or msg_type == 'reply':
                        logger.info(f"[{task_id}] Received reply from {msg_from}")
                        return msg

            await asyncio.sleep(POLL_INTERVAL)
            elapsed += POLL_INTERVAL

        except Exception as e:
            logger.debug(f"[{task_id}] Poll error: {e}")
            await asyncio.sleep(POLL_INTERVAL)
            elapsed += POLL_INTERVAL

    return None


async def execute_via_mayor_chat(
    task: str,
    context: Optional[str] = None,
    timeout: int = DEFAULT_TASK_TIMEOUT
) -> Dict[str, Any]:
    """
    Execute task via mail + nudge to Mayor (interactive Mayor session).

    Flow:
    1. Clear overseer inbox (remove stale messages)
    2. Send task via gt nudge mayor (synchronous tmux message)
    3. Poll overseer inbox for Mayor's reply
    4. Return response to caller

    This uses the existing Mayor tmux session, preserving context
    from previous conversations. Preferred when Mayor is running.
    """
    task_id = str(uuid.uuid4())[:8]
    thread_id = f"mcp-{task_id}"

    # Record the task
    TASKS[task_id] = TaskRecord(
        task_id=task_id,
        prompt=task,
        status="running"
    )

    # Build the full prompt
    prompt = task
    if context:
        prompt = f"{task}\n\nContext:\n{context}"

    # Add reply instruction for the Mayor
    prompt_with_reply = f"""{prompt}

[MCP Task ID: {task_id}]
Please reply to {MCP_IDENTITY} when complete with your response."""

    logger.info(f"[{task_id}] Sending task to Mayor via nudge: {task[:100]}...")

    try:
        # Step 1: Clear overseer inbox
        await clear_overseer_inbox()

        # Step 2: Send message to Mayor via nudge
        proc = await asyncio.create_subprocess_exec(
            'gt', 'nudge', 'mayor', prompt_with_reply,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=GT_ROOT
        )
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=NUDGE_TIMEOUT)

        if proc.returncode != 0:
            error = stderr.decode().strip() or stdout.decode().strip()
            TASKS[task_id].status = "failed"
            TASKS[task_id].error = f"Failed to nudge Mayor: {error}"
            TASKS[task_id].completed_at = datetime.now(timezone.utc)

            logger.error(f"[{task_id}] Failed to nudge Mayor: {error}")

            return {
                "success": False,
                "task_id": task_id,
                "status": "failed",
                "error": f"Failed to send task to Mayor: {error}",
                "mode": "interactive"
            }

        logger.info(f"[{task_id}] Task sent to Mayor, waiting for reply...")

        # Step 3: Poll for reply
        reply = await poll_for_reply(task_id, thread_id, timeout)

        if reply:
            response_text = reply.get('body', '') or reply.get('subject', 'Task completed')

            TASKS[task_id].status = "completed"
            TASKS[task_id].result = response_text
            TASKS[task_id].completed_at = datetime.now(timezone.utc)

            logger.info(f"[{task_id}] Task completed via mayor nudge+mail")

            # Archive the reply message
            reply_id = reply.get('id')
            if reply_id:
                archive_proc = await asyncio.create_subprocess_exec(
                    'gt', 'mail', 'archive', reply_id,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.PIPE,
                    cwd=GT_ROOT
                )
                await archive_proc.communicate()

            return {
                "success": True,
                "task_id": task_id,
                "status": "completed",
                "response": response_text,
                "mode": "interactive"
            }
        else:
            # No reply received within timeout
            TASKS[task_id].status = "timeout"
            TASKS[task_id].error = f"No reply from Mayor within {timeout}s"
            TASKS[task_id].completed_at = datetime.now(timezone.utc)

            logger.warning(f"[{task_id}] No reply from Mayor within timeout")

            return {
                "success": False,
                "task_id": task_id,
                "status": "timeout",
                "error": f"Mayor did not reply within {timeout} seconds. The task may still be processing - check Gastown terminal for status.",
                "mode": "interactive"
            }

    except asyncio.TimeoutError:
        TASKS[task_id].status = "failed"
        TASKS[task_id].error = f"Timeout after {timeout}s"
        TASKS[task_id].completed_at = datetime.now(timezone.utc)

        logger.error(f"[{task_id}] Task timed out via mayor nudge after {timeout}s")

        return {
            "success": False,
            "task_id": task_id,
            "status": "timeout",
            "error": f"Task timed out after {timeout} seconds",
            "mode": "interactive"
        }

    except Exception as e:
        TASKS[task_id].status = "failed"
        TASKS[task_id].error = str(e)
        TASKS[task_id].completed_at = datetime.now(timezone.utc)

        logger.exception(f"[{task_id}] Task failed via mayor nudge with exception")

        return {
            "success": False,
            "task_id": task_id,
            "status": "error",
            "error": str(e),
            "mode": "interactive"
        }


async def execute_via_claude_oneshot(
    task: str,
    context: Optional[str] = None,
    timeout: int = DEFAULT_TASK_TIMEOUT
) -> Dict[str, Any]:
    """
    Execute task via `claude -p` (one-shot mode).

    Fallback when Mayor session is not running. Spawns a new Claude
    process for each task - stateless but always available.
    """
    task_id = str(uuid.uuid4())[:8]
    mayor_dir = os.path.join(GT_ROOT, 'mayor')

    # Record the task
    TASKS[task_id] = TaskRecord(
        task_id=task_id,
        prompt=task,
        status="running"
    )

    # Build the full prompt
    prompt = task
    if context:
        prompt = f"{task}\n\nContext:\n{context}"

    logger.info(f"[{task_id}] Sending task to Claude (one-shot): {task[:100]}...")

    try:
        # Run Claude with the task as a one-shot prompt
        model = os.environ.get('CLAUDE_MODEL', 'claude-opus-4-5-20251101')
        proc = await asyncio.create_subprocess_exec(
            'claude', '-p', prompt,
            '--model', model,
            '--output-format', 'text',
            '--dangerously-skip-permissions',
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=mayor_dir
        )

        stdout, stderr = await asyncio.wait_for(
            proc.communicate(),
            timeout=timeout
        )

        output = stdout.decode().strip()
        error = stderr.decode().strip()

        if proc.returncode == 0:
            TASKS[task_id].status = "completed"
            TASKS[task_id].result = output
            TASKS[task_id].completed_at = datetime.now(timezone.utc)

            logger.info(f"[{task_id}] Task completed via one-shot")

            return {
                "success": True,
                "task_id": task_id,
                "status": "completed",
                "response": output,
                "mode": "oneshot"
            }
        else:
            TASKS[task_id].status = "failed"
            TASKS[task_id].error = error or f"Exit code: {proc.returncode}"
            TASKS[task_id].completed_at = datetime.now(timezone.utc)

            logger.error(f"[{task_id}] Task failed via one-shot: {error}")

            return {
                "success": False,
                "task_id": task_id,
                "status": "failed",
                "error": error or output or f"Claude exited with code {proc.returncode}",
                "mode": "oneshot"
            }

    except asyncio.TimeoutError:
        TASKS[task_id].status = "failed"
        TASKS[task_id].error = f"Timeout after {timeout}s"
        TASKS[task_id].completed_at = datetime.now(timezone.utc)

        logger.error(f"[{task_id}] Task timed out via one-shot after {timeout}s")

        return {
            "success": False,
            "task_id": task_id,
            "status": "timeout",
            "error": f"Task timed out after {timeout} seconds",
            "mode": "oneshot"
        }

    except Exception as e:
        TASKS[task_id].status = "failed"
        TASKS[task_id].error = str(e)
        TASKS[task_id].completed_at = datetime.now(timezone.utc)

        logger.exception(f"[{task_id}] Task failed via one-shot with exception")

        return {
            "success": False,
            "task_id": task_id,
            "status": "error",
            "error": str(e),
            "mode": "oneshot"
        }


async def execute_mayor_task(
    task: str,
    context: Optional[str] = None,
    wait: bool = True,
    timeout: int = DEFAULT_TASK_TIMEOUT
) -> Dict[str, Any]:
    """
    Execute a task via the Mayor (Claude Code agent).

    Execution strategy:
    1. Check if Mayor session is running (gt mayor status)
    2. If running: Use `gt mayor chat` for stateful interaction
    3. If not running: Fall back to `claude -p` one-shot mode

    The Mayor runs in ~/gt/mayor and has full access to Gastown tools (gt, bd).
    """
    # Cap timeout
    timeout = min(timeout, MAX_TASK_TIMEOUT)

    if not wait:
        # Async mode: start task in background, return immediately
        # TODO: Implement background task execution with status polling
        task_id = str(uuid.uuid4())[:8]
        TASKS[task_id] = TaskRecord(
            task_id=task_id,
            prompt=task,
            status="running"
        )
        return {
            "success": True,
            "task_id": task_id,
            "status": "running",
            "message": "Task started. Note: Async mode not fully implemented yet. Use wait=true for now."
        }

    # Check if Mayor session is running
    mayor_running = await check_mayor_session_running()

    if mayor_running:
        logger.info("Mayor session is running - using interactive mode (gt mayor chat)")
        return await execute_via_mayor_chat(task, context, timeout)
    else:
        logger.info("Mayor session not running - using one-shot mode (claude -p)")
        return await execute_via_claude_oneshot(task, context, timeout)


async def handle_tool_call(name: str, arguments: Dict[str, Any]) -> Dict[str, Any]:
    """Route tool calls to the Mayor."""

    if name == "mayor_task":
        return await execute_mayor_task(
            task=arguments.get("task", ""),
            context=arguments.get("context"),
            wait=arguments.get("wait", True),
            timeout=arguments.get("timeout", DEFAULT_TASK_TIMEOUT)
        )
    else:
        return {"error": f"Unknown tool: {name}"}


async def mcp_handler(request: web.Request) -> web.Response:
    """Handle MCP JSON-RPC requests."""

    # Read body
    body = await request.text()

    # Verify signature (if configured)
    signature = request.headers.get('X-Gastown-Signature', '')
    timestamp = request.headers.get('X-Gastown-Timestamp', '')

    if INSTANCE_TOKEN and not verify_signature(body, signature, timestamp):
        return web.json_response({
            "jsonrpc": "2.0",
            "id": None,
            "error": {"code": -32600, "message": "Invalid signature"}
        }, status=401)

    # Parse JSON-RPC request
    try:
        rpc_request = json.loads(body)
    except json.JSONDecodeError as e:
        return web.json_response({
            "jsonrpc": "2.0",
            "id": None,
            "error": {"code": -32700, "message": f"Parse error: {e}"}
        }, status=400)

    request_id = rpc_request.get("id")
    method = rpc_request.get("method", "")
    params = rpc_request.get("params", {})

    logger.info(f"MCP request: method={method}, id={request_id}")

    # Handle MCP methods
    if method == "tools/list":
        return web.json_response({
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {"tools": TOOLS}
        })

    elif method == "tools/call":
        tool_name = params.get("name", "")
        arguments = params.get("arguments", {})

        try:
            result = await handle_tool_call(tool_name, arguments)

            # Format as MCP tool result
            is_error = "error" in result and result.get("success") is not True

            return web.json_response({
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [
                        {"type": "text", "text": json.dumps(result, indent=2)}
                    ],
                    "isError": is_error
                }
            })
        except Exception as e:
            logger.exception(f"Tool execution failed: {e}")
            return web.json_response({
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [{"type": "text", "text": f"Error: {e}"}],
                    "isError": True
                }
            })

    elif method == "initialize":
        # MCP initialization handshake
        return web.json_response({
            "jsonrpc": "2.0",
            "id": request_id,
            "result": {
                "protocolVersion": PROTOCOL_VERSION,
                "serverInfo": {
                    "name": "gastown-mcp",
                    "version": SERVER_VERSION
                },
                "capabilities": {
                    "tools": {}
                }
            }
        })

    else:
        return web.json_response({
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": -32601, "message": f"Method not found: {method}"}
        })


async def health_handler(request: web.Request) -> web.Response:
    """Health check endpoint."""
    # Check if Mayor session is running for status
    mayor_running = await check_mayor_session_running()

    return web.json_response({
        "status": "ok",
        "service": "gastown-mcp",
        "gt_root": GT_ROOT,
        "active_tasks": len([t for t in TASKS.values() if t.status == "running"]),
        "execution_mode": "interactive" if mayor_running else "oneshot",
        "mayor_session": "running" if mayor_running else "stopped"
    })


def create_app() -> web.Application:
    """Create the aiohttp application."""
    app = web.Application()
    app.router.add_post('/mcp', mcp_handler)
    app.router.add_get('/health', health_handler)
    return app


def main():
    """Main entry point."""
    # Load instance token from file if not in env
    global INSTANCE_TOKEN
    if not INSTANCE_TOKEN and os.path.exists(INSTANCE_TOKEN_FILE):
        with open(INSTANCE_TOKEN_FILE) as f:
            INSTANCE_TOKEN = f.read().strip()

    logger.info(f"Starting Gastown MCP Server on port {MCP_PORT}")
    logger.info(f"GT_ROOT: {GT_ROOT}")
    logger.info(f"Token configured: {bool(INSTANCE_TOKEN)}")

    app = create_app()
    web.run_app(app, host=MCP_HOST, port=MCP_PORT, print=False)


if __name__ == '__main__':
    main()
