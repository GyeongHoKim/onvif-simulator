import { useMemo, useState } from "react"
import { RiDeleteBinLine } from "@remixicon/react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { useSim, type LogEntry } from "@/store/simulator"

function clockTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return "--:--:--"
  return d.toTimeString().slice(0, 8)
}

function levelVariant(level: string): "default" | "secondary" | "destructive" | "outline" {
  switch (level) {
    case "ERROR":
      return "destructive"
    case "WARN":
      return "outline"
    case "DEBUG":
      return "secondary"
    default:
      return "default"
  }
}

export function LogScreen() {
  const log = useSim((s) => s.log)
  const clearLog = useSim((s) => s.clearLog)

  const [showEvents, setShowEvents] = useState(true)
  const [showMutations, setShowMutations] = useState(true)
  const [showLogs, setShowLogs] = useState(true)
  const [search, setSearch] = useState("")

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return log.filter((e) => {
      if (e.kind === "event" && !showEvents) return false
      if (e.kind === "mutation" && !showMutations) return false
      if (e.kind === "log" && !showLogs) return false
      if (!q) return true
      const hay =
        e.kind === "event"
          ? `${e.topic} ${e.source} ${e.payload}`
          : e.kind === "mutation"
            ? `${e.op} ${e.target} ${e.detail}`
            : `${e.level} ${e.component} ${e.message} ${JSON.stringify(e.attrs)}`
      return hay.toLowerCase().includes(q)
    })
  }, [log, showEvents, showMutations, showLogs, search])

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Log</h1>
        <Button variant="outline" size="sm" onClick={clearLog}>
          <RiDeleteBinLine data-icon="inline-start" />
          Clear
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-4">
        <div className="flex items-center gap-2">
          <Checkbox
            id="filter-events"
            checked={showEvents}
            onCheckedChange={(v) => setShowEvents(Boolean(v))}
          />
          <Label htmlFor="filter-events">Events</Label>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id="filter-mutations"
            checked={showMutations}
            onCheckedChange={(v) => setShowMutations(Boolean(v))}
          />
          <Label htmlFor="filter-mutations">Mutations</Label>
        </div>
        <div className="flex items-center gap-2">
          <Checkbox
            id="filter-logs"
            checked={showLogs}
            onCheckedChange={(v) => setShowLogs(Boolean(v))}
          />
          <Label htmlFor="filter-logs">Logs</Label>
        </div>
        <Input
          placeholder="Search…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="max-w-xs"
        />
      </div>

      {filtered.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No log entries</EmptyTitle>
            <EmptyDescription>
              Trigger an event or mutate config to populate the log.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-24">Time</TableHead>
              <TableHead className="w-28">Kind</TableHead>
              <TableHead>Target</TableHead>
              <TableHead>Detail</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.map((e, i) => (
              <LogRow key={i} entry={e} />
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

function logDetailLine(entry: Extract<LogEntry, { kind: "log" }>): string {
  const bits: string[] = [entry.message]
  for (const [k, v] of Object.entries(entry.attrs)) {
    if (k === "request_id") continue
    const s = typeof v === "string" ? v : JSON.stringify(v)
    bits.push(`${k}=${s}`)
  }
  return bits.join(" ")
}

function LogRow({ entry }: { entry: LogEntry }) {
  if (entry.kind === "event") {
    return (
      <TableRow>
        <TableCell className="font-mono text-xs">{clockTime(entry.time)}</TableCell>
        <TableCell>
          <Badge variant="secondary">event</Badge>{" "}
          <span className="font-mono text-xs">{entry.topic.replace(/^tns1:/, "")}</span>
        </TableCell>
        <TableCell className="font-mono text-xs">{entry.source || "—"}</TableCell>
        <TableCell className="font-mono text-xs">{entry.payload}</TableCell>
      </TableRow>
    )
  }
  if (entry.kind === "mutation") {
    return (
      <TableRow>
        <TableCell className="font-mono text-xs">{clockTime(entry.time)}</TableCell>
        <TableCell>
          <Badge>mutation</Badge>{" "}
          <span className="font-mono text-xs">{entry.op}</span>
        </TableCell>
        <TableCell className="font-mono text-xs">{entry.target || "—"}</TableCell>
        <TableCell className="font-mono text-xs">{entry.detail}</TableCell>
      </TableRow>
    )
  }

  const rid = entry.attrs.request_id
  const targetCol =
    rid !== undefined && rid !== null && String(rid) !== "" ? String(rid).slice(-4) : "—"

  return (
    <TableRow>
      <TableCell className="font-mono text-xs">{clockTime(entry.time)}</TableCell>
      <TableCell>
        <Badge
          variant={levelVariant(entry.level)}
          className={
            entry.level === "WARN"
              ? "border-amber-500/50 text-amber-800 dark:border-amber-500/40 dark:text-amber-400"
              : undefined
          }
        >
          {entry.level}
        </Badge>{" "}
        <span className="font-mono text-xs">{entry.component || "log"}</span>
      </TableCell>
      <TableCell className="font-mono text-xs">{targetCol}</TableCell>
      <TableCell className="font-mono text-xs break-all">{logDetailLine(entry)}</TableCell>
    </TableRow>
  )
}
