Name:           aiconnect
Version:        %{?tagver}%{!?tagver:0.0.0}
Release:        1%{?dist}
Summary:        Reverse proxy per AI backends con autenticazione AD

License:        Proprietary
URL:            https://github.com/fzanti/aiconnect
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang
BuildRequires:  shadow-utils

Requires(pre):  shadow-utils
Requires(post): systemd
Requires(preun): systemd
Requires(postun): systemd

%description
AIConnect è un reverse proxy HTTPS in Go per instradare richieste AI verso backend multipli
(Ollama, OpenAI, vLLM) con autenticazione Active Directory, load balancing e metriche Prometheus.

%prep
%autosetup -n %{name}-%{version}

%build
export CGO_ENABLED=0
go build -trimpath -ldflags "-s -w" -o %{name} ./cmd/aiconnect

%install
install -Dpm0755 %{name} %{buildroot}/usr/local/bin/%{name}

# systemd unit
install -Dpm0644 deployment/aiconnect.service %{buildroot}/usr/lib/systemd/system/aiconnect.service

# default config (do not overwrite local changes)
install -Dpm0600 config.example.yaml %{buildroot}/etc/aiconnect/config.yaml

# autocert cache directory
install -d -m0700 %{buildroot}/var/cache/aiconnect/autocert

%pre
getent group aiconnect >/dev/null || groupadd -r aiconnect
getent passwd aiconnect >/dev/null || useradd -r -g aiconnect -s /sbin/nologin -d /var/cache/aiconnect aiconnect
exit 0

%post
# ensure cache directory ownership for autocert
mkdir -p /var/cache/aiconnect/autocert
chown -R aiconnect:aiconnect /var/cache/aiconnect
chmod 700 /var/cache/aiconnect/autocert

systemctl daemon-reload >/dev/null 2>&1 || :
systemctl preset aiconnect.service >/dev/null 2>&1 || :

%preun
if [ "$1" -eq 0 ]; then
	systemctl --no-reload disable --now aiconnect.service >/dev/null 2>&1 || :
fi

%postun
systemctl daemon-reload >/dev/null 2>&1 || :

%files
%doc README.md
/usr/local/bin/%{name}
%config(noreplace) %attr(0600,root,root) /etc/aiconnect/config.yaml
/usr/lib/systemd/system/aiconnect.service
%dir %attr(0700,aiconnect,aiconnect) /var/cache/aiconnect
%dir %attr(0700,aiconnect,aiconnect) /var/cache/aiconnect/autocert

%changelog
* Sat Dec 13 2025 AIConnect CI <ci@example.invalid> - %{version}-1
- Pacchetto iniziale
