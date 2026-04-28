export namespace config {
	
	export class JWTConfig {
	    enabled?: boolean;
	    issuer?: string;
	    audience?: string;
	    jwks_url?: string;
	    public_key_pem?: string[];
	    algorithms?: string[];
	    username_claim?: string;
	    roles_claim?: string;
	    clock_skew?: string;
	    require_tls?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JWTConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.issuer = source["issuer"];
	        this.audience = source["audience"];
	        this.jwks_url = source["jwks_url"];
	        this.public_key_pem = source["public_key_pem"];
	        this.algorithms = source["algorithms"];
	        this.username_claim = source["username_claim"];
	        this.roles_claim = source["roles_claim"];
	        this.clock_skew = source["clock_skew"];
	        this.require_tls = source["require_tls"];
	    }
	}
	export class DigestConfig {
	    realm?: string;
	    algorithms?: string[];
	    nonce_ttl?: string;
	
	    static createFrom(source: any = {}) {
	        return new DigestConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.realm = source["realm"];
	        this.algorithms = source["algorithms"];
	        this.nonce_ttl = source["nonce_ttl"];
	    }
	}
	export class UserConfig {
	    username: string;
	    password: string;
	    role: string;
	
	    static createFrom(source: any = {}) {
	        return new UserConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.password = source["password"];
	        this.role = source["role"];
	    }
	}
	export class AuthConfig {
	    enabled: boolean;
	    users?: UserConfig[];
	    digest?: DigestConfig;
	    jwt?: JWTConfig;
	
	    static createFrom(source: any = {}) {
	        return new AuthConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.users = this.convertValues(source["users"], UserConfig);
	        this.digest = this.convertValues(source["digest"], DigestConfig);
	        this.jwt = this.convertValues(source["jwt"], JWTConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SystemDateTimeConfig {
	    date_time_type?: string;
	    daylight_savings?: boolean;
	    tz?: string;
	    manual_date_time_utc?: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemDateTimeConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date_time_type = source["date_time_type"];
	        this.daylight_savings = source["daylight_savings"];
	        this.tz = source["tz"];
	        this.manual_date_time_utc = source["manual_date_time_utc"];
	    }
	}
	export class NetworkInterfaceIPv4 {
	    enabled: boolean;
	    dhcp: boolean;
	    manual?: string[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkInterfaceIPv4(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.dhcp = source["dhcp"];
	        this.manual = source["manual"];
	    }
	}
	export class NetworkInterfaceConfig {
	    token: string;
	    enabled: boolean;
	    hw_address?: string;
	    mtu?: number;
	    ipv4?: NetworkInterfaceIPv4;
	
	    static createFrom(source: any = {}) {
	        return new NetworkInterfaceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.enabled = source["enabled"];
	        this.hw_address = source["hw_address"];
	        this.mtu = source["mtu"];
	        this.ipv4 = this.convertValues(source["ipv4"], NetworkInterfaceIPv4);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NetworkProtocol {
	    name: string;
	    enabled: boolean;
	    port?: number[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkProtocol(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.port = source["port"];
	    }
	}
	export class DefaultGatewayConfig {
	    ipv4_address?: string[];
	    ipv6_address?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DefaultGatewayConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ipv4_address = source["ipv4_address"];
	        this.ipv6_address = source["ipv6_address"];
	    }
	}
	export class DNSConfig {
	    from_dhcp?: boolean;
	    search_domain?: string[];
	    dns_manual?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DNSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from_dhcp = source["from_dhcp"];
	        this.search_domain = source["search_domain"];
	        this.dns_manual = source["dns_manual"];
	    }
	}
	export class RuntimeConfig {
	    discovery_mode?: string;
	    hostname?: string;
	    dns?: DNSConfig;
	    default_gateway?: DefaultGatewayConfig;
	    network_protocols?: NetworkProtocol[];
	    network_interfaces?: NetworkInterfaceConfig[];
	    system_date_and_time?: SystemDateTimeConfig;
	
	    static createFrom(source: any = {}) {
	        return new RuntimeConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.discovery_mode = source["discovery_mode"];
	        this.hostname = source["hostname"];
	        this.dns = this.convertValues(source["dns"], DNSConfig);
	        this.default_gateway = this.convertValues(source["default_gateway"], DefaultGatewayConfig);
	        this.network_protocols = this.convertValues(source["network_protocols"], NetworkProtocol);
	        this.network_interfaces = this.convertValues(source["network_interfaces"], NetworkInterfaceConfig);
	        this.system_date_and_time = this.convertValues(source["system_date_and_time"], SystemDateTimeConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TopicConfig {
	    name: string;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TopicConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	    }
	}
	export class EventsConfig {
	    max_pull_points?: number;
	    subscription_timeout?: string;
	    topics?: TopicConfig[];
	
	    static createFrom(source: any = {}) {
	        return new EventsConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_pull_points = source["max_pull_points"];
	        this.subscription_timeout = source["subscription_timeout"];
	        this.topics = this.convertValues(source["topics"], TopicConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MetadataConfig {
	    token: string;
	    name: string;
	    analytics?: boolean;
	    ptz_status?: boolean;
	    events?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MetadataConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.token = source["token"];
	        this.name = source["name"];
	        this.analytics = source["analytics"];
	        this.ptz_status = source["ptz_status"];
	        this.events = source["events"];
	    }
	}
	export class ProfileConfig {
	    name: string;
	    token: string;
	    media_file_path?: string;
	    encoding?: string;
	    width?: number;
	    height?: number;
	    fps?: number;
	    bitrate?: number;
	    gop_length?: number;
	    snapshot_uri?: string;
	    video_source_token?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.token = source["token"];
	        this.media_file_path = source["media_file_path"];
	        this.encoding = source["encoding"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.bitrate = source["bitrate"];
	        this.gop_length = source["gop_length"];
	        this.snapshot_uri = source["snapshot_uri"];
	        this.video_source_token = source["video_source_token"];
	    }
	}
	export class MediaConfig {
	    profiles: ProfileConfig[];
	    max_video_encoder_instances?: number;
	    metadata_configurations?: MetadataConfig[];
	
	    static createFrom(source: any = {}) {
	        return new MediaConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profiles = this.convertValues(source["profiles"], ProfileConfig);
	        this.max_video_encoder_instances = source["max_video_encoder_instances"];
	        this.metadata_configurations = this.convertValues(source["metadata_configurations"], MetadataConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class NetworkConfig {
	    http_port: number;
	    rtsp_port?: number;
	    interface?: string;
	    xaddrs?: string[];
	
	    static createFrom(source: any = {}) {
	        return new NetworkConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.http_port = source["http_port"];
	        this.rtsp_port = source["rtsp_port"];
	        this.interface = source["interface"];
	        this.xaddrs = source["xaddrs"];
	    }
	}
	export class DeviceConfig {
	    uuid: string;
	    manufacturer: string;
	    model: string;
	    serial: string;
	    firmware?: string;
	    scopes?: string[];
	
	    static createFrom(source: any = {}) {
	        return new DeviceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.uuid = source["uuid"];
	        this.manufacturer = source["manufacturer"];
	        this.model = source["model"];
	        this.serial = source["serial"];
	        this.firmware = source["firmware"];
	        this.scopes = source["scopes"];
	    }
	}
	export class Config {
	    version: number;
	    device: DeviceConfig;
	    network: NetworkConfig;
	    media: MediaConfig;
	    auth?: AuthConfig;
	    events?: EventsConfig;
	    runtime?: RuntimeConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.device = this.convertValues(source["device"], DeviceConfig);
	        this.network = this.convertValues(source["network"], NetworkConfig);
	        this.media = this.convertValues(source["media"], MediaConfig);
	        this.auth = this.convertValues(source["auth"], AuthConfig);
	        this.events = this.convertValues(source["events"], EventsConfig);
	        this.runtime = this.convertValues(source["runtime"], RuntimeConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	
	

}

export namespace gui {
	
	export class EventRecord {
	    // Go type: time
	    time: any;
	    topic: string;
	    source: string;
	    payload: string;
	
	    static createFrom(source: any = {}) {
	        return new EventRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.topic = source["topic"];
	        this.source = source["source"];
	        this.payload = source["payload"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogEntry {
	    // Go type: time
	    time: any;
	    kind: string;
	    topic?: string;
	    source?: string;
	    payload?: string;
	    op?: string;
	    target?: string;
	    detail?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.kind = source["kind"];
	        this.topic = source["topic"];
	        this.source = source["source"];
	        this.payload = source["payload"];
	        this.op = source["op"];
	        this.target = source["target"];
	        this.detail = source["detail"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogPage {
	    entries: LogEntry[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new LogPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], LogEntry);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Status {
	    running: boolean;
	    listenAddr: string;
	    // Go type: time
	    startedAt: any;
	    uptime: number;
	    discoveryMode: string;
	    profileCount: number;
	    topicCount: number;
	    userCount: number;
	    activePullSubs: number;
	    recentEvents: EventRecord[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.listenAddr = source["listenAddr"];
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.uptime = source["uptime"];
	        this.discoveryMode = source["discoveryMode"];
	        this.profileCount = source["profileCount"];
	        this.topicCount = source["topicCount"];
	        this.userCount = source["userCount"];
	        this.activePullSubs = source["activePullSubs"];
	        this.recentEvents = this.convertValues(source["recentEvents"], EventRecord);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UserView {
	    username: string;
	    roles: string[];
	
	    static createFrom(source: any = {}) {
	        return new UserView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.roles = source["roles"];
	    }
	}

}

