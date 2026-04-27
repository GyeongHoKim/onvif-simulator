import { useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Field, FieldGroup, FieldLabel, FieldError } from "@/components/ui/field"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { useSim } from "@/store/simulator"
import * as App from "@/lib/wails/wailsjs/go/gui/App"
import { config as cfgNs } from "@/lib/wails/wailsjs/go/models"

type Config = cfgNs.Config

export function DeviceScreen() {
  const config = useSim((s) => s.config)
  if (!config) return null

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-semibold tracking-tight">Device</h1>

      <IdentityCard config={config} />
      <NetworkCard
        port={config.network?.http_port ?? 0}
        discoveryMode={config.runtime?.discovery_mode ?? "Discoverable"}
        hostname={config.runtime?.hostname ?? ""}
      />
      <RuntimeCard config={config} />
    </div>
  )
}

function IdentityCard({ config }: { config: Config }) {
  const d = config.device
  return (
    <Card>
      <CardHeader>
        <CardTitle>Identity</CardTitle>
      </CardHeader>
      <CardContent className="grid grid-cols-2 gap-4 text-sm">
        <KV label="UUID" value={d.uuid ?? "—"} mono />
        <KV label="Firmware" value={d.firmware ?? "—"} />
        <KV label="Manufacturer" value={d.manufacturer ?? "—"} />
        <KV label="Model" value={d.model ?? "—"} />
        <KV label="Serial" value={d.serial ?? "—"} />
        <div className="col-span-2">
          <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
            Scopes
          </div>
          <div className="flex flex-wrap gap-1">
            {(d.scopes ?? []).map((s) => (
              <Badge key={s} variant="secondary" className="font-mono text-xs">
                {s}
              </Badge>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function NetworkCard({
  port,
  discoveryMode,
  hostname,
}: {
  port: number
  discoveryMode: string
  hostname: string
}) {
  const [host, setHost] = useState(hostname)
  const [hostErr, setHostErr] = useState<string | null>(null)

  async function saveHostname() {
    setHostErr(null)
    try {
      await App.SetHostname(host)
      toast.success("Hostname saved")
      await useSim.getState().refreshConfig()
    } catch (e) {
      setHostErr(String(e))
    }
  }

  async function setMode(mode: "Discoverable" | "NonDiscoverable") {
    try {
      await App.SetDiscoveryMode(mode)
      toast.success(`Discovery: ${mode}`)
      await useSim.getState().refreshConfig()
    } catch (e) {
      toast.error(String(e))
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Network</CardTitle>
      </CardHeader>
      <CardContent>
        <FieldGroup>
          <Field>
            <FieldLabel>HTTP port</FieldLabel>
            <Input value={String(port)} disabled className="font-mono" />
            <span className="text-xs text-muted-foreground">
              Changing the port requires a restart.
            </span>
          </Field>

          <Field>
            <FieldLabel>Discovery mode</FieldLabel>
            <div className="inline-flex rounded-md border p-0.5">
              {(["Discoverable", "NonDiscoverable"] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => void setMode(m)}
                  className={cn(
                    "rounded-sm px-3 py-1 text-sm transition-colors",
                    discoveryMode === m
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-muted"
                  )}
                >
                  {m}
                </button>
              ))}
            </div>
          </Field>

          <Field data-invalid={hostErr ? true : undefined}>
            <FieldLabel>Hostname</FieldLabel>
            <div className="flex gap-2">
              <Input value={host} onChange={(e) => setHost(e.target.value)} />
              <Button onClick={() => void saveHostname()}>Save</Button>
            </div>
            {hostErr && <FieldError>{hostErr}</FieldError>}
          </Field>
        </FieldGroup>
      </CardContent>
    </Card>
  )
}

function RuntimeCard({ config }: { config: Config }) {
  const r = config.runtime ?? ({} as NonNullable<Config["runtime"]>)
  return (
    <Card>
      <CardHeader>
        <CardTitle>Runtime (read-only)</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 text-sm">
        <div>
          <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
            DNS
          </div>
          <KV label="From DHCP" value={r.dns?.from_dhcp ? "yes" : "no"} />
          <KV
            label="Search domains"
            value={(r.dns?.search_domain ?? []).join(", ") || "—"}
          />
          <KV
            label="Manual"
            value={(r.dns?.dns_manual ?? []).join(", ") || "—"}
          />
        </div>
        <div>
          <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
            Default gateway
          </div>
          <KV
            label="IPv4"
            value={(r.default_gateway?.ipv4_address ?? []).join(", ") || "—"}
          />
          <KV
            label="IPv6"
            value={(r.default_gateway?.ipv6_address ?? []).join(", ") || "—"}
          />
        </div>
        <div>
          <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
            Network protocols
          </div>
          {(r.network_protocols ?? []).length === 0 ? (
            <span className="text-muted-foreground">—</span>
          ) : (
            (r.network_protocols ?? []).map((p) => (
              <KV
                key={p.name}
                label={p.name ?? ""}
                value={`${p.enabled ? "on" : "off"} · ports ${(p.port ?? []).join(", ") || "—"}`}
              />
            ))
          )}
        </div>
        <div>
          <div className="mb-1 text-xs font-medium uppercase text-muted-foreground">
            System date/time
          </div>
          <KV label="Type" value={r.system_date_and_time?.date_time_type || "—"} />
          <KV
            label="Daylight savings"
            value={r.system_date_and_time?.daylight_savings ? "yes" : "no"}
          />
          <KV label="Timezone" value={r.system_date_and_time?.tz || "—"} />
        </div>
        <p className="text-xs text-muted-foreground">
          These fields are written by ONVIF clients via Set* operations and
          mirrored here for visibility only.
        </p>
      </CardContent>
    </Card>
  )
}

function KV({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex gap-2 text-sm">
      <span className="w-40 shrink-0 text-muted-foreground">{label}</span>
      <span className={cn("truncate", mono && "font-mono")}>{value}</span>
    </div>
  )
}
