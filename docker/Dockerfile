FROM ubuntu:focal

# set environment variables for tzdata
ARG TZ=America/New_York
ENV TZ=${TZ}

# include manual pages and documentation
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update &&\
  yes | unminimize

# install GCC-related packages
RUN apt-get update && apt-get -y install\
  build-essential\
  binutils-doc\
  cpp-doc\
  gcc-doc\
  g++\
  g++-multilib\
  gdb\
  gdb-doc\
  glibc-doc\
  libblas-dev\
  liblapack-dev\
  liblapack-doc\
  libstdc++-10-doc\
  make\
  make-doc

# install clang-related packages
RUN apt-get -y install\
  clang\
  clang-10-doc\
  lldb\
  clang-format

# install programs used for system exploration
RUN apt-get -y install\
  blktrace\
  linux-tools-generic\
  strace\
  tcpdump\
  htop

# install interactive programs (emacs, vim, nano, man, sudo, etc.)
RUN apt-get -y install\
  bc\
  curl\
  dc\
  git\
  git-doc\
  man\
  micro\
  nano\
  psmisc\
  sudo\
  wget\
  screen\
  tmux\
  emacs-nox\
  vim 

# install rust
RUN RUSTUP_HOME=/opt/rust CARGO_HOME=/opt/rust \
  bash -c "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sudo -E sh -s -- -y"

# Install golang
RUN bash -c "mkdir /usr/local/go && wget -O - https://go.dev/dl/go1.19.1.linux-amd64.tar.gz | sudo tar -xvz -C /usr/local"

# set up libraries
RUN apt-get -y install\
  libreadline-dev\
  locales\
  wamerican\
  libssl-dev

# install programs used for networking
RUN apt-get -y install\
  dnsutils\
  inetutils-ping\
  iproute2\
  net-tools\
  netcat\
  telnet\
  time\
  pv\
  traceroute

# set up default locale
RUN locale-gen en_US.UTF-8
ENV LANG en_US.UTF-8

COPY ./snowcast-autograder /home/cs1680/snowcast-autograder

# set up passwordless sudo for user cs1680-user
RUN useradd -m -s /bin/bash cs1680-user && \
  echo "cs1680-user ALL=(ALL:ALL) NOPASSWD: ALL" > /etc/sudoers.d/cs1680-init

RUN usermod -a -G www-data cs1680-user

# create binary reporting version of dockerfile
RUN (echo '#\!/bin/sh'; echo 'echo 1') > /usr/bin/cs1680-docker-version; chmod ugo+rx,u+w,go-w /usr/bin/cs1680-docker-version

# git build arguments
ARG USER=cs1680\ User
ARG EMAIL=nobody@example.com

# configure your environment
USER cs1680-user
RUN rm -f ~/.bash_logout

WORKDIR /home/cs1680
CMD ["/bin/bash", "-l"]
