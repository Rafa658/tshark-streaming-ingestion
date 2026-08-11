import argparse
from ingestor.sources import LiveTsharkSource, PcapReplaySource
from ingestor.publisher import MQTTPublisher


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", choices=["live", "pcap"], required=True)
    parser.add_argument("--interface", default=ingestor_settings.TSHARK_INTERFACE)
    parser.add_argument("--bpf-filter", default=ingestor_settings.TSHARK_BPF_FILTER)
    parser.add_argument("--pcap", default=ingestor_settings.TSHARK_PCAP_PATH)
    parser.add_argument("--replay-loop", action="store_true")
    args = parser.parse_args()

    publisher = MQTTPublisher()
    publisher.start()

    if args.source == "live":
        source = LiveTsharkSource(args.interface, args.bpf_filter)
    else:
        source = PcapReplaySource(args.pcap, args.replay_loop)

    for line in source.lines():
        publisher.publish_line(line)

    publisher.stop()


if __name__ == "__main__":
    from tshark_shared.config import ingestor_settings
    main()