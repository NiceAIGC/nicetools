import { useEffect, useRef, useState } from "react";
import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Checkbox,
  Chip,
  Divider,
  Input,
  Progress,
  Select,
  SelectItem,
  Switch,
} from "@heroui/react";

type Alarm = {
  id: string;
  time: string;
  label: string;
  enabled: boolean;
  days: number[];
  snoozeAt?: string;
  lastTriggered?: string;
};

const storageKey = "nicetools.alarms";
const weekdays = ["日", "一", "二", "三", "四", "五", "六"];

const hours = Array.from({ length: 24 }, (_, value) => String(value).padStart(2, "0"));
const minutes = Array.from({ length: 60 }, (_, value) => String(value).padStart(2, "0"));

function loadAlarms(): Alarm[] {
  try {
    const value = localStorage.getItem(storageKey);
    return value ? (JSON.parse(value) as Alarm[]) : [];
  } catch {
    return [];
  }
}

function formatTime(date: Date) {
  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(date);
}

function dateKey(date: Date) {
  return `${date.getFullYear()}-${date.getMonth() + 1}-${date.getDate()}`;
}

function nextAlarmTime(alarm: Alarm, now: Date): Date | undefined {
  if (!alarm.enabled) return undefined;
  if (alarm.snoozeAt) return new Date(alarm.snoozeAt);

  const [hours, minutes] = alarm.time.split(":").map(Number);
  for (let offset = 0; offset < 7; offset += 1) {
    const candidate = new Date(now);
    candidate.setHours(hours, minutes, 0, 0);
    candidate.setDate(candidate.getDate() + offset);
    if (candidate <= now) continue;
    if (alarm.days.length === 0 || alarm.days.includes(candidate.getDay())) return candidate;
  }
  return undefined;
}

export default function AlarmClock() {
  const [alarms, setAlarms] = useState<Alarm[]>(loadAlarms);
  const [now, setNow] = useState(() => new Date());
  const [time, setTime] = useState("07:00");
  const [label, setLabel] = useState("");
  const [days, setDays] = useState<number[]>([]);
  const [ringing, setRinging] = useState<Alarm>();
  const [audioReady, setAudioReady] = useState(false);
  const [audioStatus, setAudioStatus] = useState("请先授权声音，闹钟才能播放提示音。");
  const audioContext = useRef<AudioContext>();
  const beepTimer = useRef<number>();
  const alarmsRef = useRef(alarms);
  const ringingRef = useRef(ringing);

  useEffect(() => {
    alarmsRef.current = alarms;
    localStorage.setItem(storageKey, JSON.stringify(alarms));
  }, [alarms]);

  useEffect(() => {
    ringingRef.current = ringing;
  }, [ringing]);

  function beep() {
    const context = audioContext.current;
    if (!context || context.state !== "running") return false;
    const oscillator = context.createOscillator();
    const gain = context.createGain();
    oscillator.frequency.value = 880;
    gain.gain.setValueAtTime(0.0001, context.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.2, context.currentTime + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, context.currentTime + 0.3);
    oscillator.connect(gain).connect(context.destination);
    oscillator.start();
    oscillator.stop(context.currentTime + 0.32);
    return true;
  }

  function startRinging(alarm: Alarm) {
    if (ringingRef.current) return;
    setRinging(alarm);
    if (!beep()) setAudioStatus("闹钟已触发，但声音未授权；点击“授权闹钟声音”后可播放。");
    beepTimer.current = window.setInterval(beep, 850);
    if ("Notification" in window && Notification.permission === "granted") {
      new Notification(alarm.label || "闹钟提醒", { body: `${alarm.time} 时间到了` });
    }
  }

  function stopSound() {
    if (beepTimer.current) window.clearInterval(beepTimer.current);
    beepTimer.current = undefined;
  }

  function dismiss() {
    const active = ringingRef.current;
    stopSound();
    setRinging(undefined);
    if (!active) return;
    setAlarms((current) =>
      current.map((alarm) =>
        alarm.id === active.id
          ? { ...alarm, snoozeAt: undefined, enabled: alarm.days.length > 0 ? alarm.enabled : false }
          : alarm,
      ),
    );
  }

  function snooze() {
    const active = ringingRef.current;
    stopSound();
    setRinging(undefined);
    if (!active) return;
    setAlarms((current) =>
      current.map((alarm) =>
        alarm.id === active.id
          ? { ...alarm, snoozeAt: new Date(Date.now() + 5 * 60_000).toISOString() }
          : alarm,
      ),
    );
  }

  async function enableAudio() {
    const Context = window.AudioContext || (window as typeof window & { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!Context) {
      setAudioStatus("当前浏览器不支持闹钟声音。");
      return;
    }
    try {
      audioContext.current ??= new Context();
      await audioContext.current.resume();
      const ready = audioContext.current.state === "running";
      setAudioReady(ready);
      setAudioStatus(ready ? "声音已授权，已播放试听提示音。" : "浏览器阻止了声音播放，请再次点击授权。");
      if (ready) beep();
      if ("Notification" in window && Notification.permission === "default") {
        await Notification.requestPermission();
      }
    } catch {
      setAudioReady(false);
      setAudioStatus("声音授权失败，请检查浏览器的自动播放权限。");
    }
  }

  useEffect(() => {
    const timer = window.setInterval(() => {
      const current = new Date();
      setNow(current);
      if (ringingRef.current) return;
      const currentTime = current.toTimeString().slice(0, 5);
      const today = dateKey(current);
      const due = alarmsRef.current.find((alarm) => {
        if (!alarm.enabled) return false;
        if (alarm.snoozeAt) return new Date(alarm.snoozeAt).getTime() <= current.getTime();
        if (alarm.lastTriggered === today) return false;
        return alarm.time === currentTime && (alarm.days.length === 0 || alarm.days.includes(current.getDay()));
      });
      if (!due) return;
      setAlarms((currentAlarms) =>
        currentAlarms.map((alarm) =>
          alarm.id === due.id ? { ...alarm, lastTriggered: today, snoozeAt: undefined } : alarm,
        ),
      );
      startRinging(due);
    }, 1_000);
    return () => {
      window.clearInterval(timer);
      stopSound();
    };
  }, []);

  function addAlarm() {
    setAlarms((current) => [
      ...current,
      { id: crypto.randomUUID(), time, label: label.trim(), enabled: true, days },
    ]);
    setLabel("");
    setDays([]);
  }

  function addRelativeAlarm(minutes: number) {
    const target = new Date(Date.now() + minutes * 60_000);
    const targetTime = target.toTimeString().slice(0, 5);
    setAlarms((current) => [
      ...current,
      {
        id: crypto.randomUUID(),
        time: targetTime,
        label: `${minutes} 分钟后提醒`,
        enabled: true,
        days: [],
      },
    ]);
  }

  function updateAlarm(id: string, update: Partial<Alarm>) {
    setAlarms((current) => current.map((alarm) => (alarm.id === id ? { ...alarm, ...update } : alarm)));
  }

  const next = alarms
    .map((alarm) => ({ alarm, date: nextAlarmTime(alarm, now) }))
    .filter((item): item is { alarm: Alarm; date: Date } => Boolean(item.date))
    .sort((a, b) => a.date.getTime() - b.date.getTime())[0];

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <Card shadow="sm" className="border border-default-200">
        <CardBody className="gap-5 p-5 sm:p-6">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <p className="text-sm text-default-500">设备本地时间</p>
              <p className="text-4xl font-semibold tabular-nums tracking-tight text-foreground sm:text-5xl">{formatTime(now)}</p>
            </div>
            <div className="flex flex-col items-start gap-1 sm:items-end">
              <Button color={audioReady ? "success" : "primary"} variant={audioReady ? "flat" : "solid"} onPress={enableAudio}>
                {audioReady ? "声音已授权 · 点击试听" : "授权闹钟声音"}
              </Button>
              <p className="text-xs text-default-500">{audioStatus}</p>
            </div>
          </div>
          <Divider />
          <div className="rounded-large bg-primary-50 p-4 text-primary-700 dark:text-primary-300">
            <p className="text-sm font-medium">下一个闹钟</p>
            {next ? (
              <p className="mt-1 text-lg">{next.date.toLocaleString("zh-CN", { hour: "2-digit", minute: "2-digit", weekday: "short" })}{next.alarm.label ? ` · ${next.alarm.label}` : ""}</p>
            ) : (
              <p className="mt-1 text-sm">暂无已启用闹钟</p>
            )}
          </div>
        </CardBody>
      </Card>

      <Card shadow="sm" className="border border-default-200">
        <CardHeader className="pb-0"><h2 className="text-lg font-semibold">添加闹钟</h2></CardHeader>
        <CardBody className="gap-4">
          <div className="grid gap-3 sm:grid-cols-[150px_150px_1fr_auto]">
            <Select aria-label="小时" label="时间" selectedKeys={[time.slice(0, 2)]} onSelectionChange={(keys) => {
              const hour = [...keys][0];
              if (typeof hour === "string") setTime(`${hour}:${time.slice(3, 5)}`);
            }}>
              {hours.map((hour) => <SelectItem key={hour}>{hour} 时</SelectItem>)}
            </Select>
            <Select aria-label="分钟" label="分钟" selectedKeys={[time.slice(3, 5)]} onSelectionChange={(keys) => {
              const minute = [...keys][0];
              if (typeof minute === "string") setTime(`${time.slice(0, 2)}:${minute}`);
            }}>
              {minutes.map((minute) => <SelectItem key={minute}>{minute} 分</SelectItem>)}
            </Select>
            <Input aria-label="闹钟备注" label="备注（可选）" placeholder="例如：起床、开会" value={label} onValueChange={setLabel} />
            <Button color="primary" onPress={addAlarm}>添加闹钟</Button>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm text-default-500">快捷添加</span>
            {[15, 30, 60, 120].map((minutes) => (
              <Button key={minutes} size="sm" variant="flat" onPress={() => addRelativeAlarm(minutes)}>
                {minutes < 60 ? `${minutes} 分钟后` : `${minutes / 60} 小时后`}
              </Button>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            {weekdays.map((day, index) => (
              <Checkbox key={day} isSelected={days.includes(index)} onValueChange={(selected) => setDays((current) => selected ? [...current, index] : current.filter((value) => value !== index))}>
                周{day}
              </Checkbox>
            ))}
          </div>
          <p className="text-xs text-default-500">不选重复日期表示仅提醒一次；浏览器需要保持打开，声音须先授权。</p>
        </CardBody>
      </Card>

      <Card shadow="sm" className="border border-default-200">
        <CardHeader className="justify-between pb-0">
          <h2 className="text-lg font-semibold">我的闹钟</h2>
          <Chip size="sm" variant="flat">{alarms.length} 个闹钟</Chip>
        </CardHeader>
        <CardBody className="gap-3">
          {alarms.length === 0 ? <p className="py-6 text-center text-sm text-default-500">还没有闹钟，添加一个开始使用。</p> : alarms.map((alarm) => (
            <div key={alarm.id} className="flex flex-col gap-3 rounded-large border border-default-200 p-4 sm:flex-row sm:items-center">
              <p className="text-2xl font-semibold tabular-nums">{alarm.time}</p>
              <div className="min-w-0 flex-1"><p className="truncate text-sm">{alarm.label || "闹钟提醒"}</p><p className="mt-1 text-xs text-default-500">{alarm.snoozeAt ? `贪睡至 ${formatTime(new Date(alarm.snoozeAt))}` : alarm.days.length ? `每周${alarm.days.map((day) => weekdays[day]).join("、")}` : "仅一次"}</p></div>
              <Switch size="sm" isSelected={alarm.enabled} onValueChange={(enabled) => updateAlarm(alarm.id, { enabled })}>启用</Switch>
              <Button size="sm" color="danger" variant="light" onPress={() => setAlarms((current) => current.filter((item) => item.id !== alarm.id))}>删除</Button>
            </div>
          ))}
        </CardBody>
      </Card>

      {ringing && (
        <Card className="fixed inset-x-4 bottom-4 z-50 mx-auto max-w-md border-2 border-danger bg-background shadow-large">
          <CardBody className="items-center gap-3 text-center">
            <p className="text-sm text-danger">时间到了</p><p className="text-4xl font-bold tabular-nums">{ringing.time}</p><p>{ringing.label || "闹钟提醒"}</p>
            <Progress isIndeterminate aria-label="闹钟正在响" color="danger" className="mt-1" />
            <div className="flex gap-3"><Button variant="flat" onPress={snooze}>贪睡 5 分钟</Button><Button color="danger" onPress={dismiss}>停止响铃</Button></div>
          </CardBody>
        </Card>
      )}
    </div>
  );
}
