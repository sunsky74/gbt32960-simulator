export namespace bridge {
	
	export class AppSettings {
	    consoleExportCap: number;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.consoleExportCap = source["consoleExportCap"];
	    }
	}
	export class ConnectionConfig {
	    name: string;
	    host: string;
	    port: number;
	    version: string;
	    vin: string;
	    iccid: string;
	    subsystemCodes: string[];
	    heartbeatSec: number;
	    autoClockSync: boolean;
	    autoReconnect: boolean;
	    reportInterval: number;
	    reissueOffsetSec: number;
	    platformMode: boolean;
	    platformVin?: string;
	    platformUser?: string;
	    platformPass?: string;
	    signatureType: number;
	    signatureR?: string;
	    signatureS?: string;
	    extensionPack?: string;
	    tls: tlsconf.Config;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.version = source["version"];
	        this.vin = source["vin"];
	        this.iccid = source["iccid"];
	        this.subsystemCodes = source["subsystemCodes"];
	        this.heartbeatSec = source["heartbeatSec"];
	        this.autoClockSync = source["autoClockSync"];
	        this.autoReconnect = source["autoReconnect"];
	        this.reportInterval = source["reportInterval"];
	        this.reissueOffsetSec = source["reissueOffsetSec"];
	        this.platformMode = source["platformMode"];
	        this.platformVin = source["platformVin"];
	        this.platformUser = source["platformUser"];
	        this.platformPass = source["platformPass"];
	        this.signatureType = source["signatureType"];
	        this.signatureR = source["signatureR"];
	        this.signatureS = source["signatureS"];
	        this.extensionPack = source["extensionPack"];
	        this.tls = this.convertValues(source["tls"], tlsconf.Config);
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
	export class ExportResult {
	    path: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.count = source["count"];
	    }
	}
	export class ExtCommandInfo {
	    packId: string;
	    packLabel: string;
	    key: string;
	    label: string;
	    code: number;
	    respType: string;
	    fields: schema.FieldSchema[];
	    defaults: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new ExtCommandInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.packId = source["packId"];
	        this.packLabel = source["packLabel"];
	        this.key = source["key"];
	        this.label = source["label"];
	        this.code = source["code"];
	        this.respType = source["respType"];
	        this.fields = this.convertValues(source["fields"], schema.FieldSchema);
	        this.defaults = source["defaults"];
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
	export class PackInfo {
	    id: string;
	    label: string;
	    vendor?: string;
	    baseVersion: string;
	    unitCount: number;
	    commandCount: number;
	    enabled: boolean;
	    scope?: string[];
	
	    static createFrom(source: any = {}) {
	        return new PackInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.vendor = source["vendor"];
	        this.baseVersion = source["baseVersion"];
	        this.unitCount = source["unitCount"];
	        this.commandCount = source["commandCount"];
	        this.enabled = source["enabled"];
	        this.scope = source["scope"];
	    }
	}
	export class PreviewResult {
	    cmd: string;
	    hex: string;
	    len: number;
	
	    static createFrom(source: any = {}) {
	        return new PreviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cmd = source["cmd"];
	        this.hex = source["hex"];
	        this.len = source["len"];
	    }
	}
	export class ProfileSummary {
	    name: string;
	    host: string;
	    port: number;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ProfileSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.active = source["active"];
	    }
	}
	export class ReportState {
	    on: boolean;
	    intervalSec: number;
	
	    static createFrom(source: any = {}) {
	        return new ReportState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.on = source["on"];
	        this.intervalSec = source["intervalSec"];
	    }
	}
	export class ServerConfig {
	    ip: string;
	    port: number;
	    idleEnabled: boolean;
	    idleSeconds: number;
	    maxConns: number;
	    maxFrameBytes: number;
	    logLines: number;
	    maxVinsPerConn: number;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ip = source["ip"];
	        this.port = source["port"];
	        this.idleEnabled = source["idleEnabled"];
	        this.idleSeconds = source["idleSeconds"];
	        this.maxConns = source["maxConns"];
	        this.maxFrameBytes = source["maxFrameBytes"];
	        this.logLines = source["logLines"];
	        this.maxVinsPerConn = source["maxVinsPerConn"];
	    }
	}
	export class TestResult {
	    ok: boolean;
	    elapsedMs: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.elapsedMs = source["elapsedMs"];
	        this.message = source["message"];
	    }
	}
	export class TrackInfo {
	    name: string;
	    format: string;
	    count: number;
	    skipped: number;
	
	    static createFrom(source: any = {}) {
	        return new TrackInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.format = source["format"];
	        this.count = source["count"];
	        this.skipped = source["skipped"];
	    }
	}
	export class TrackReplayStatus {
	    running: boolean;
	    index: number;
	    total: number;
	    loop: boolean;
	    lastError: string;
	    lng: number;
	    lat: number;
	
	    static createFrom(source: any = {}) {
	        return new TrackReplayStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.index = source["index"];
	        this.total = source["total"];
	        this.loop = source["loop"];
	        this.lastError = source["lastError"];
	        this.lng = source["lng"];
	        this.lat = source["lat"];
	    }
	}

}

export namespace engine {
	
	export class ParamOption {
	    value: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class ParamResponseRow {
	    id: number;
	    hex: string;
	
	    static createFrom(source: any = {}) {
	        return new ParamResponseRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hex = source["hex"];
	    }
	}
	export class ParamSpec {
	    id: number;
	    name: string;
	    type: string;
	    length: number;
	    rawMin?: number;
	    rawMax?: number;
	    note?: string;
	    options?: ParamOption[];
	
	    static createFrom(source: any = {}) {
	        return new ParamSpec(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.length = source["length"];
	        this.rawMin = source["rawMin"];
	        this.rawMax = source["rawMax"];
	        this.note = source["note"];
	        this.options = this.convertValues(source["options"], ParamOption);
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
	export class ResponseCode {
	    code: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ResponseCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.label = source["label"];
	    }
	}

}

export namespace parser {
	
	export class ByteIssue {
	    start: number;
	    end: number;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new ByteIssue(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.note = source["note"];
	    }
	}
	export class Field {
	    offset: number;
	    length: number;
	    name: string;
	    type: string;
	    rawHex: string;
	    rawValue: string;
	    offsetVal: string;
	    translate: string;
	    unit: string;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new Field(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.offset = source["offset"];
	        this.length = source["length"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.rawHex = source["rawHex"];
	        this.rawValue = source["rawValue"];
	        this.offsetVal = source["offsetVal"];
	        this.translate = source["translate"];
	        this.unit = source["unit"];
	        this.note = source["note"];
	    }
	}
	export class Result {
	    totalBytes: number;
	    version: string;
	    versionByte: string;
	    command: string;
	    responseType: string;
	    vin: string;
	    encryption: string;
	    payloadLen: number;
	    fields: Field[];
	    warnings: string[];
	    issues?: ByteIssue[];
	    tree?: any;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.totalBytes = source["totalBytes"];
	        this.version = source["version"];
	        this.versionByte = source["versionByte"];
	        this.command = source["command"];
	        this.responseType = source["responseType"];
	        this.vin = source["vin"];
	        this.encryption = source["encryption"];
	        this.payloadLen = source["payloadLen"];
	        this.fields = this.convertValues(source["fields"], Field);
	        this.warnings = source["warnings"];
	        this.issues = this.convertValues(source["issues"], ByteIssue);
	        this.tree = source["tree"];
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

export namespace schema {
	
	export class BitDef {
	    index: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new BitDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.label = source["label"];
	    }
	}
	export class EnumDef {
	    value: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new EnumDef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
	    }
	}
	export class FieldSchema {
	    key: string;
	    label: string;
	    kind: string;
	    unit?: string;
	    min?: number;
	    max?: number;
	    enum?: EnumDef[];
	    bits?: BitDef[];
	    itemLabel?: string;
	    minItems?: number;
	    scaleNote?: string;
	    length?: number;
	
	    static createFrom(source: any = {}) {
	        return new FieldSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.unit = source["unit"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.enum = this.convertValues(source["enum"], EnumDef);
	        this.bits = this.convertValues(source["bits"], BitDef);
	        this.itemLabel = source["itemLabel"];
	        this.minItems = source["minItems"];
	        this.scaleNote = source["scaleNote"];
	        this.length = source["length"];
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
	export class GroupSchema {
	    key: string;
	    title: string;
	    enabled: boolean;
	    multiple: boolean;
	    maxRows?: number;
	    fields: FieldSchema[];
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new GroupSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.title = source["title"];
	        this.enabled = source["enabled"];
	        this.multiple = source["multiple"];
	        this.maxRows = source["maxRows"];
	        this.fields = this.convertValues(source["fields"], FieldSchema);
	        this.source = source["source"];
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
	export class NamedGroup {
	    key: string;
	    enabled: boolean;
	    rows: any[];
	
	    static createFrom(source: any = {}) {
	        return new NamedGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.enabled = source["enabled"];
	        this.rows = source["rows"];
	    }
	}
	export class GroupsPayload {
	    groups: NamedGroup[];
	
	    static createFrom(source: any = {}) {
	        return new GroupsPayload(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], NamedGroup);
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

export namespace servermode {
	
	export class Session {
	    vin: string;
	    peer: string;
	    // Go type: time
	    loginAt: any;
	    // Go type: time
	    lastSeen: any;
	    rxCount: number;
	    txCount: number;
	    platform?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Session(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.vin = source["vin"];
	        this.peer = source["peer"];
	        this.loginAt = this.convertValues(source["loginAt"], null);
	        this.lastSeen = this.convertValues(source["lastSeen"], null);
	        this.rxCount = source["rxCount"];
	        this.txCount = source["txCount"];
	        this.platform = source["platform"];
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
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.listenAddr = source["listenAddr"];
	    }
	}

}

export namespace tlsconf {
	
	export class Config {
	    enabled: boolean;
	    ca: string;
	    clientCert: string;
	    clientKey: string;
	    serverName: string;
	    insecure: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.ca = source["ca"];
	        this.clientCert = source["clientCert"];
	        this.clientKey = source["clientKey"];
	        this.serverName = source["serverName"];
	        this.insecure = source["insecure"];
	    }
	}

}

export namespace updater {
	
	export class DownloadResult {
	    tag: string;
	    assetName: string;
	    size: number;
	    sha256: string;
	
	    static createFrom(source: any = {}) {
	        return new DownloadResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.assetName = source["assetName"];
	        this.size = source["size"];
	        this.sha256 = source["sha256"];
	    }
	}
	export class UpdateInfo {
	    current: string;
	    latest: string;
	    hasUpdate: boolean;
	    devBuild: boolean;
	    assetName: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.hasUpdate = source["hasUpdate"];
	        this.devBuild = source["devBuild"];
	        this.assetName = source["assetName"];
	    }
	}

}

