import { useMemo, useState } from "react"
import { RiRefreshLine, RiCodeLine } from "@remixicon/react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field"
import { Empty, EmptyHeader, EmptyTitle, EmptyDescription } from "@/components/ui/empty"
import { cn } from "@/lib/utils"
import { useSim } from "@/store/simulator"
import * as App from "@/lib/wails/wailsjs/go/main/App"

type TriggerKind = "motion" | "blurry" | "dark" | "bright" | "digital" | "raw"

function classifyTopic(name: string): TriggerKind {
  if (name.endsWith("/MotionAlarm")) return "motion"
  if (name.endsWith("/ImageTooBlurry")) return "blurry"
  if (name.endsWith("/ImageTooDark")) return "dark"
  if (name.endsWith("/ImageTooBright")) return "bright"
  if (name.endsWith("/DigitalInput")) return "digital"
  return "raw"
}

export function EventsScreen() {
  const config = useSim((s) => s.config)
  const running = useSim((s) => s.status?.running ?? false)

  const topics = config?.events?.topics ?? []
  const firstSourceToken = useMemo(() => {
    const p = config?.media?.profiles ?? []
    return p.find((x) => x.video_source_token)?.video_source_token || "VS_DEFAULT"
  }, [config])

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-baseline justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Events</h1>
        <span className="text-sm text-muted-foreground">
          {topics.length} topic{topics.length === 1 ? "" : "s"} advertised
        </span>
      </div>

      {topics.length === 0 ? (
        <Empty>
          <EmptyHeader>
            <EmptyTitle>No topics configured</EmptyTitle>
            <EmptyDescription>
              Add entries to <code>events.topics</code> in onvif-simulator.json.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[40%]">Topic</TableHead>
              <TableHead className="w-24">Enabled</TableHead>
              <TableHead>Trigger</TableHead>
              <TableHead className="w-20 text-right">Sync</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {topics.map((t) => (
              <TopicRow
                key={t.name ?? ""}
                name={t.name ?? ""}
                enabled={t.enabled ?? false}
                running={running}
                defaultSource={firstSourceToken ?? "VS_DEFAULT"}
              />
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

type TopicRowProps = {
  name: string
  enabled: boolean
  running: boolean
  defaultSource: string
}

function TopicRow({ name, enabled, running, defaultSource }: TopicRowProps) {
  const kind = classifyTopic(name)
  const dimmed = !enabled
  const interactable = enabled && running

  async function onToggle(next: boolean) {
    try {
      await App.SetTopicEnabled(name, next)
      toast.success(`${shortName(name)} ${next ? "enabled" : "disabled"}`)
      await useSim.getState().refreshConfig()
    } catch (e) {
      toast.error(String(e))
    }
  }

  return (
    <TableRow className={cn(dimmed && "opacity-60")}>
      <TableCell className="font-mono text-xs">{name}</TableCell>
      <TableCell>
        <Checkbox checked={enabled} onCheckedChange={(v) => void onToggle(Boolean(v))} />
      </TableCell>
      <TableCell>
        {kind === "motion" || kind === "blurry" || kind === "dark" || kind === "bright" ? (
          <VideoSourceTrigger
            topic={name}
            kind={kind}
            disabled={!interactable}
            defaultSource={defaultSource}
          />
        ) : kind === "digital" ? (
          <DigitalInputTrigger disabled={!interactable} />
        ) : (
          <RawPublishTrigger topic={name} disabled={!enabled} />
        )}
      </TableCell>
      <TableCell className="text-right">
        <SyncPopover topic={name} disabled={!enabled} defaultSource={defaultSource} />
      </TableCell>
    </TableRow>
  )
}

function shortName(t: string): string {
  return t.replace(/^tns1:/, "")
}

function VideoSourceTrigger({
  topic,
  kind,
  disabled,
  defaultSource,
}: {
  topic: string
  kind: "motion" | "blurry" | "dark" | "bright"
  disabled: boolean
  defaultSource: string
}) {
  const [token, setToken] = useState(defaultSource)

  async function trigger(state: boolean) {
    try {
      if (kind === "motion") await App.Motion(token, state)
      else if (kind === "blurry") await App.ImageTooBlurry(token, state)
      else if (kind === "dark") await App.ImageTooDark(token, state)
      else await App.ImageTooBright(token, state)
      toast.success(
        `${shortName(topic)} sent to ${token || "(empty)"}: ${state ? "ON" : "OFF"}`
      )
    } catch (e) {
      toast.error(String(e))
    }
  }

  return (
    <div className="flex items-center gap-2">
      <Input
        className="h-8 w-40 font-mono text-xs"
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="source token"
        disabled={disabled}
      />
      <Button size="sm" disabled={disabled} onClick={() => void trigger(true)}>
        On
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => void trigger(false)}
      >
        Off
      </Button>
    </div>
  )
}

function DigitalInputTrigger({ disabled }: { disabled: boolean }) {
  const [token, setToken] = useState("DI_0")

  async function trigger(state: boolean) {
    try {
      await App.DigitalInput(token, state)
      toast.success(`DigitalInput ${token}: ${state ? "HIGH" : "LOW"}`)
    } catch (e) {
      toast.error(String(e))
    }
  }

  return (
    <div className="flex items-center gap-2">
      <Input
        className="h-8 w-40 font-mono text-xs"
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="input token"
        disabled={disabled}
      />
      <Button size="sm" disabled={disabled} onClick={() => void trigger(true)}>
        On
      </Button>
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => void trigger(false)}
      >
        Off
      </Button>
    </div>
  )
}

function RawPublishTrigger({ topic, disabled }: { topic: string; disabled: boolean }) {
  const [open, setOpen] = useState(false)
  const [xml, setXml] = useState("")

  async function submit() {
    try {
      await App.PublishRaw(topic, xml)
      toast.success(`Raw XML published to ${shortName(topic)}`)
      setOpen(false)
    } catch (e) {
      toast.error(String(e))
    }
  }

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        <RiCodeLine data-icon="inline-start" />
        Publish raw XML
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Publish raw XML</DialogTitle>
            <DialogDescription>
              Topic: <code className="font-mono">{topic}</code>. No validation —
              the body is passed verbatim into the broker.
            </DialogDescription>
          </DialogHeader>
          <Textarea
            value={xml}
            onChange={(e) => setXml(e.target.value)}
            rows={12}
            className="font-mono text-xs"
            placeholder="<tt:Message UtcTime=&quot;...&quot;>...</tt:Message>"
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button onClick={() => void submit()}>Publish</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function SyncPopover({
  topic,
  disabled,
  defaultSource,
}: {
  topic: string
  disabled: boolean
  defaultSource: string
}) {
  const [sourceItem, setSourceItem] = useState("VideoSourceToken")
  const [source, setSource] = useState(defaultSource)
  const [dataItem, setDataItem] = useState("State")
  const [state, setState] = useState(true)

  async function submit() {
    try {
      await App.SyncProperty(topic, sourceItem, source, dataItem, state)
      toast.success(`${shortName(topic)} synchronized`)
    } catch (e) {
      toast.error(String(e))
    }
  }

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={disabled}
            aria-label={`Sync ${shortName(topic)}`}
          >
            <RiRefreshLine />
          </Button>
        }
      />
      <PopoverContent className="w-80">
        <FieldGroup>
          <Field>
            <FieldLabel>Source item name</FieldLabel>
            <Input
              value={sourceItem}
              onChange={(e) => setSourceItem(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>Source token</FieldLabel>
            <Input value={source} onChange={(e) => setSource(e.target.value)} />
          </Field>
          <Field>
            <FieldLabel>Data item name</FieldLabel>
            <Input
              value={dataItem}
              onChange={(e) => setDataItem(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel>State</FieldLabel>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant={state ? "default" : "outline"}
                onClick={() => setState(true)}
              >
                true
              </Button>
              <Button
                size="sm"
                variant={!state ? "default" : "outline"}
                onClick={() => setState(false)}
              >
                false
              </Button>
            </div>
          </Field>
          <Button onClick={() => void submit()}>Send SyncProperty</Button>
        </FieldGroup>
      </PopoverContent>
    </Popover>
  )
}
