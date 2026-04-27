import { useEffect } from "react"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { useSim, startStatusPolling } from "@/store/simulator"

function shortTopic(t: string): string {
  return t.replace(/^tns1:/, "")
}

function clockTime(iso: string | Date): string {
  const d = typeof iso === "string" ? new Date(iso) : iso
  if (isNaN(d.getTime())) return "--:--:--"
  return d.toTimeString().slice(0, 8)
}

export function DashboardScreen() {
  const status = useSim((s) => s.status)

  useEffect(() => startStatusPolling(1000), [])

  const running = status?.running ?? false
  const recent = status?.recentEvents ?? []

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Stat label="Running" value={running ? "Yes" : "No"} tone={running ? "good" : "muted"} />
        <Stat label="Listen address" value={status?.listenAddr || "—"} />
        <Stat label="Uptime" value={formatUptime(Number(status?.uptime ?? 0))} />
        <Stat label="Discovery" value={status?.discoveryMode || "—"} />
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
        <Stat label="Profiles" value={String(status?.profileCount ?? 0)} />
        <Stat label="Enabled topics" value={String(status?.topicCount ?? 0)} />
        <Stat label="Users" value={String(status?.userCount ?? 0)} />
        <Stat label="Pull subscriptions" value={String(status?.activePullSubs ?? 0)} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Recent events</CardTitle>
        </CardHeader>
        <CardContent>
          {recent.length === 0 ? (
            <Empty>
              <EmptyHeader>
                <EmptyTitle>No recent events</EmptyTitle>
                <EmptyDescription>
                  Trigger one from the Events screen.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-24">Time</TableHead>
                  <TableHead>Topic</TableHead>
                  <TableHead>Source</TableHead>
                  <TableHead>State</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {recent.slice(0, 20).map((e, i) => (
                  <TableRow key={i}>
                    <TableCell className="font-mono text-xs">{clockTime(e.time)}</TableCell>
                    <TableCell className="font-mono text-xs">{shortTopic(e.topic)}</TableCell>
                    <TableCell className="font-mono text-xs">{e.source || "—"}</TableCell>
                    <TableCell className="font-mono text-xs">{e.payload}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function Stat({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: "good" | "muted"
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div
          className={
            tone === "good"
              ? "text-2xl font-semibold text-emerald-600 dark:text-emerald-400"
              : tone === "muted"
                ? "text-2xl font-semibold text-muted-foreground"
                : "text-2xl font-semibold"
          }
        >
          {value}
        </div>
      </CardContent>
    </Card>
  )
}

function formatUptime(ns: number): string {
  const totalSec = Math.floor(ns / 1e9)
  if (totalSec <= 0) return "—"
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = totalSec % 60
  return `${h}h ${m}m ${s}s`
}
